package room

import (
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"echo/internal/player"
	"echo/internal/protocol"
)

// ════════════════════════════════════════════════════════════════
//  Room — 一局对战的容器
// ════════════════════════════════════════════════════════════════

// Room 持有参与一局对战的两名玩家，以及（Phase 4 后）游戏引擎。
//
// 座位（Seat）约定：
//   Seat 0 = 先手（先行动）
//   Seat 1 = 后手
//
// 设计原则：Room 只负责"谁在这局"和"这局的生命周期"，
// 具体的游戏逻辑交给 Phase 4 的 game.Engine。
type Room struct {
	ID      string
	Players [2]*player.Player // Players[0] 先手，Players[1] 后手

	mu        sync.Mutex
	createdAt time.Time

	// 人机对战配置：AISeat=-1 表示 PvP，>=0 表示该座位由 AI 控制。
	AISeat   int    // -1 = PvP; 0 or 1 = AI seat
	AICharID string // 仅 AISeat >= 0 时有效，AI 预选角色 ID
}

// Broadcast 向房间内所有玩家发送相同消息。
// 用于：场地效果公告、回合开始通知等不区分玩家的事件。
func (r *Room) Broadcast(msgID uint16, payload []byte) {
	for _, p := range r.Players {
		p.Send(msgID, payload)
	}
}

// SendTo 向指定座位的玩家发送消息。
// 用于：发送该玩家专属的状态视图（信息遮蔽后的 GameStateView）。
func (r *Room) SendTo(seat int, msgID uint16, payload []byte) {
	if seat < 0 || seat >= len(r.Players) {
		return
	}
	r.Players[seat].Send(msgID, payload)
}

// OpponentSeat 返回指定座位的对手座位编号。
func (r *Room) OpponentSeat(seat int) int {
	return 1 - seat
}

// SeatOf 返回指定玩家在本房间的座位编号，找不到返回 -1。
func (r *Room) SeatOf(playerID string) int {
	for seat, p := range r.Players {
		if p.ID == playerID {
			return seat
		}
	}
	return -1
}

// ════════════════════════════════════════════════════════════════
//  Manager — 房间生命周期管理
// ════════════════════════════════════════════════════════════════

var roomIDCounter atomic.Uint64

// Manager 创建、存储、销毁所有活跃房间。
type Manager struct {
	rooms sync.Map // roomID → *Room

	// onCreatedHooks 在每次房间创建后异步调用（游戏引擎在此注入）。
	hookMu         sync.RWMutex
	onCreatedHooks []func(*Room)
}

// NewManager 创建房间管理器。
func NewManager() *Manager {
	return &Manager{}
}

// OnRoomCreated 注册一个回调，在每次房间创建完成后异步调用。
// 用途：游戏引擎模块在此注册"为新房间创建 Engine"的逻辑。
func (m *Manager) OnRoomCreated(fn func(*Room)) {
	m.hookMu.Lock()
	defer m.hookMu.Unlock()
	m.onCreatedHooks = append(m.onCreatedHooks, fn)
}

// CreateRoom 为两名玩家创建一个新房间，并立即通知双方匹配成功。
// 此函数通常在新的 goroutine 中调用（由匹配队列触发）。
func (m *Manager) CreateRoom(p0, p1 *player.Player) *Room {
	r := &Room{
		ID:        fmt.Sprintf("room-%d", roomIDCounter.Add(1)),
		Players:   [2]*player.Player{p0, p1},
		createdAt: time.Now(),
		AISeat:    -1, // PvP，无 AI
	}
	m.rooms.Store(r.ID, r)

	p0.SetRoom(r.ID)
	p1.SetRoom(r.ID)

	slog.Info("room created",
		"roomID", r.ID,
		"seat0", p0.ID,
		"seat1", p1.ID,
	)

	// 触发 OnRoomCreated 钩子（游戏引擎在此初始化）
	m.hookMu.RLock()
	hooks := m.onCreatedHooks
	m.hookMu.RUnlock()
	for _, fn := range hooks {
		go fn(r) // 异步执行，不阻塞 CreateRoom
	}

	// 通知双方匹配成功
	// 注意：两个 Send 是异步的（进入各自的 sendCh），不会互相阻塞
	for seat, p := range r.Players {
		opponent := r.Players[r.OpponentSeat(seat)]
		ev := protocol.MatchFoundEv{
			GameID:       r.ID,
			YourSeat:     seat,
			OpponentName: opponent.Name,
		}
		p.Send(protocol.MsgMatchFoundEv, protocol.MustEncode(ev))
	}

	// Phase 4：在此启动游戏引擎
	// r.engine = game.NewEngine(r)
	// go r.engine.Start()

	return r
}

// CreateAIRoom 为一名玩家和一个虚拟 AI 创建房间，即时开始（无需排队）。
// 人类玩家始终坐 Seat 0（先手），AI 坐 Seat 1（后手）。
// aiCharID 是 AI 的预选角色，引擎启动后会自动完成 AI 的角色选择步骤。
func (m *Manager) CreateAIRoom(human *player.Player, aiCharID string) *Room {
	aiPlayer := player.NewAIPlayer("AI")
	r := &Room{
		ID:        fmt.Sprintf("room-%d", roomIDCounter.Add(1)),
		Players:   [2]*player.Player{human, aiPlayer},
		createdAt: time.Now(),
		AISeat:    1,
		AICharID:  aiCharID,
	}
	m.rooms.Store(r.ID, r)
	human.SetRoom(r.ID)

	slog.Info("AI room created",
		"roomID", r.ID,
		"humanSeat", 0,
		"humanID", human.ID,
		"aiCharID", aiCharID,
	)

	// 触发 OnRoomCreated 钩子（创建游戏引擎）
	m.hookMu.RLock()
	hooks := m.onCreatedHooks
	m.hookMu.RUnlock()
	for _, fn := range hooks {
		go fn(r)
	}

	// 只通知人类玩家：匹配成功，对手名为 "AI"
	human.Send(protocol.MsgMatchFoundEv, protocol.MustEncode(protocol.MatchFoundEv{
		GameID:       r.ID,
		YourSeat:     0,
		OpponentName: "AI",
	}))

	return r
}

// Get 根据 roomID 查找房间，找不到返回 nil。
func (m *Manager) Get(roomID string) *Room {
	v, ok := m.rooms.Load(roomID)
	if !ok {
		return nil
	}
	return v.(*Room)
}

// Remove 销毁房间，清理玩家的房间绑定。
// 通常在游戏结束后由游戏引擎调用。
func (m *Manager) Remove(roomID string) {
	v, ok := m.rooms.LoadAndDelete(roomID)
	if !ok {
		return
	}
	r := v.(*Room)
	for _, p := range r.Players {
		p.SetRoom("")
	}
	slog.Info("room removed", "roomID", roomID)
}

// RoomCount 返回当前活跃房间数，可用于监控。
func (m *Manager) RoomCount() int {
	count := 0
	m.rooms.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}
