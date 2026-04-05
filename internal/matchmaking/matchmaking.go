package matchmaking

import (
	"log/slog"
	"math/rand"
	"sync"

	"echo/internal/game/character"
	"echo/internal/game/field"
	"echo/internal/network"
	"echo/internal/player"
	"echo/internal/protocol"
	"echo/internal/room"
)

// ════════════════════════════════════════════════════════════════
//  Queue — 匹配等待队列
// ════════════════════════════════════════════════════════════════

// Queue 是一个线程安全的 FIFO 匹配队列。
// 每当队列中有 2 名玩家，立即配对并创建房间。
//
// 为什么用 slice + Mutex 而不是 channel？
//
//	channel 做队列无法实现"根据 ID 删除中间元素"（玩家取消匹配）。
//	slice + Mutex 可以按 ID 查找并删除，更灵活。
type Queue struct {
	mu      sync.Mutex
	waiting []*player.Player
	roomMgr *room.Manager
	rng     *rand.Rand
}

// NewQueue 创建匹配队列。
func NewQueue(roomMgr *room.Manager) *Queue {
	return &Queue{roomMgr: roomMgr, rng: rand.New(rand.NewSource(rand.Int63()))}
}

// Enqueue 将玩家加入等待队列，并尝试立即匹配。
// 若玩家已在队列中，忽略重复入队。
func (q *Queue) Enqueue(p *player.Player) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for _, existing := range q.waiting {
		if existing.ID == p.ID {
			return // 已在队列中，不重复添加
		}
	}

	q.waiting = append(q.waiting, p)
	slog.Info("player joined queue", "playerID", p.ID, "name", p.Name, "queueSize", len(q.waiting))

	q.tryMatch()
}

// Dequeue 将玩家从队列中移除（取消匹配或断线时调用）。
func (q *Queue) Dequeue(playerID string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for i, p := range q.waiting {
		if p.ID == playerID {
			// 删除第 i 个元素（保持顺序）
			q.waiting = append(q.waiting[:i], q.waiting[i+1:]...)
			slog.Info("player left queue", "playerID", playerID, "queueSize", len(q.waiting))
			return
		}
	}
}

// tryMatch 尝试从队列头部取出 2 名玩家进行配对。
// 必须在持有 q.mu 的情况下调用。
func (q *Queue) tryMatch() {
	if len(q.waiting) < 2 {
		return
	}

	p0 := q.waiting[0]
	p1 := q.waiting[1]
	q.waiting = q.waiting[2:]

	// 随机决定先后手，避免先加入队列的玩家永远先手
	if q.rng.Intn(2) == 1 {
		p0, p1 = p1, p0
	}

	slog.Info("match found", "p0", p0.ID, "p1", p1.ID)

	// 在新 goroutine 中创建房间，避免持锁期间做耗时操作。
	// 这是一个重要的设计原则：锁内只做内存操作，I/O 和耗时逻辑放到锁外。
	go q.roomMgr.CreateRoom(p0, p1)
}

// ════════════════════════════════════════════════════════════════
//  Handler — 匹配相关的消息处理器
// ════════════════════════════════════════════════════════════════

// Handler 封装匹配系统的消息处理函数，持有所有依赖。
// 比起全局变量或函数参数传递，Handler 结构体让依赖关系更清晰。
type Handler struct {
	playerMgr  *player.Manager
	queue      *Queue
	roomMgr    *room.Manager
	configHash string // 游戏配置版本哈希，启动时计算
}

// NewHandler 创建 Handler，需传入所有依赖（依赖注入）。
func NewHandler(pm *player.Manager, q *Queue, rm *room.Manager, configHash string) *Handler {
	return &Handler{playerMgr: pm, queue: q, roomMgr: rm, configHash: configHash}
}

// RegisterAll 将所有匹配相关的 handler 注册到 router。
// 在 main.go 中调用一次即可。
func (h *Handler) RegisterAll(r *network.Router) {
	r.Register(protocol.MsgLoginReq, h.handleLogin)
	r.Register(protocol.MsgJoinQueueReq, h.handleJoinQueue)
	r.Register(protocol.MsgLeaveQueueReq, h.handleLeaveQueue)
	r.Register(protocol.MsgCreateAIGameReq, h.handleCreateAIGame)
	r.Register(protocol.MsgGameConfigReq, h.handleGameConfigReq)
}

// handleLogin 处理登录请求，支持首次登录和断线重连两种路径。
func (h *Handler) handleLogin(s *network.Session, data []byte) {
	req, err := protocol.Decode[protocol.LoginReq](data)
	if err != nil {
		s.Send(protocol.MsgLoginResp, protocol.MustEncode(protocol.LoginResp{
			Success: false, Error: "invalid request",
		}))
		return
	}

	// ── 路径 A：断线重连 ──────────────────────────────────────
	if req.ReconnectToken != "" {
		p := h.playerMgr.Reconnect(req.ReconnectToken, s)
		if p == nil {
			s.Send(protocol.MsgLoginResp, protocol.MustEncode(protocol.LoginResp{
				Success: false, Error: "reconnect token invalid or expired",
			}))
			return
		}

		inGame := p.RoomID() != ""
		s.Send(protocol.MsgLoginResp, protocol.MustEncode(protocol.LoginResp{
			Success:        true,
			PlayerID:       p.ID,
			ReconnectToken: req.ReconnectToken, // 原 token 续期，客户端继续保存
			InGame:         inGame,
			ConfigHash:     h.configHash,
		}))

		// 若仍在对局中，重发 MatchFoundEv 让客户端回到游戏界面
		// Phase 4 会改为重发完整的 GameStateView
		if inGame {
			r := h.roomMgr.Get(p.RoomID())
			if r != nil {
				seat := r.SeatOf(p.ID)
				opponent := r.Players[r.OpponentSeat(seat)]
				p.Send(protocol.MsgMatchFoundEv, protocol.MustEncode(protocol.MatchFoundEv{
					GameID:       r.ID,
					YourSeat:     seat,
					OpponentName: opponent.Name,
				}))
			}
		}
		return
	}

	// ── 路径 B：首次登录 ──────────────────────────────────────
	if req.PlayerName == "" {
		s.Send(protocol.MsgLoginResp, protocol.MustEncode(protocol.LoginResp{
			Success: false, Error: "player_name is required",
		}))
		return
	}

	p, token := h.playerMgr.Register(req.PlayerName, s)
	s.Send(protocol.MsgLoginResp, protocol.MustEncode(protocol.LoginResp{
		Success:        true,
		PlayerID:       p.ID,
		ReconnectToken: token, // 客户端必须持久化这个 token！
		ConfigHash:     h.configHash,
	}))
}

// handleGameConfigReq 客户端请求完整配置（本地缓存 hash 不匹配时发送）。
func (h *Handler) handleGameConfigReq(s *network.Session, _ []byte) {
	s.Send(protocol.MsgGameConfigEv, protocol.MustEncode(protocol.GameConfigEv{
		Characters: character.AllJSON(),
		Fields:     field.AllJSON(),
		ConfigHash: h.configHash,
	}))
}

// handleJoinQueue 处理加入匹配队列请求。
func (h *Handler) handleJoinQueue(s *network.Session, data []byte) {
	p := h.playerMgr.GetBySession(s.ID)
	if p == nil {
		s.Send(protocol.MsgJoinQueueResp, protocol.MustEncode(protocol.JoinQueueResp{
			Success: false, Error: "not logged in",
		}))
		return
	}

	if p.RoomID() != "" {
		s.Send(protocol.MsgJoinQueueResp, protocol.MustEncode(protocol.JoinQueueResp{
			Success: false, Error: "already in a game",
		}))
		return
	}

	h.queue.Enqueue(p)
	s.Send(protocol.MsgJoinQueueResp, protocol.MustEncode(protocol.JoinQueueResp{
		Success: true,
	}))
}

// handleLeaveQueue 处理取消匹配请求。
func (h *Handler) handleLeaveQueue(s *network.Session, data []byte) {
	p := h.playerMgr.GetBySession(s.ID)
	if p == nil {
		return
	}
	h.queue.Dequeue(p.ID)
}

// handleCreateAIGame 处理人机对战请求，即时创建房间，无需排队等待。
// 玩家和 AI 的角色均由客户端在请求中指定。
func (h *Handler) handleCreateAIGame(s *network.Session, data []byte) {
	p := h.playerMgr.GetBySession(s.ID)
	if p == nil {
		s.Send(protocol.MsgErrorEv, protocol.MustEncode(protocol.ErrorEv{
			Code: protocol.ErrCodeInvalidPhase, Message: "请先登录",
		}))
		return
	}
	if p.RoomID() != "" {
		s.Send(protocol.MsgErrorEv, protocol.MustEncode(protocol.ErrorEv{
			Code: protocol.ErrCodeInvalidPhase, Message: "已在对局中",
		}))
		return
	}

	req, err := protocol.Decode[protocol.CreateAIGameReq](data)
	if err != nil || req.PlayerCharID == "" || req.AICharID == "" {
		s.Send(protocol.MsgErrorEv, protocol.MustEncode(protocol.ErrorEv{
			Code: protocol.ErrCodeInvalidPhase, Message: "无效的人机对战请求（需指定双方角色）",
		}))
		return
	}

	slog.Info("creating AI game", "playerID", p.ID, "playerChar", req.PlayerCharID, "aiChar", req.AICharID)

	// 创建房间（内部广播 MatchFoundEv 给人类玩家，并触发引擎初始化钩子）
	h.roomMgr.CreateAIRoom(p, req.AICharID)

	// 人类玩家无需等待对方选角，直接发送角色选择请求即可；
	// 服务端引擎会自动完成 AI 的选角，游戏随即开始。
	// 客户端在收到 MatchFoundEv 后会自动发送 SelectCharacterReq(PlayerCharID)。
	// 为了让流程与 PvP 一致，此处不做额外处理，由客户端正常走选角流程。
}
