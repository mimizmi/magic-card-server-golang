package game

import (
	"log/slog"
	"sync"

	"echo/internal/network"
	"echo/internal/player"
	"echo/internal/protocol"
	"echo/internal/room"
)

// Handler 将进来的网络消息路由到对应房间的 Engine。
//
// 关键职责：
//   1. 根据 Session 找到对应的 Player
//   2. 根据 Player 的 RoomID 找到对应的 Engine
//   3. 把消息投递给 Engine.SubmitAction
//
// 不做：不处理任何游戏逻辑（逻辑全在 Engine）。
type Handler struct {
	playerMgr *player.Manager
	roomMgr   *room.Manager
	engines   sync.Map // roomID → *Engine
}

// NewHandler 创建游戏消息处理器。
func NewHandler(pm *player.Manager, rm *room.Manager) *Handler {
	return &Handler{playerMgr: pm, roomMgr: rm}
}

// RegisterAll 注册所有游戏内消息的处理函数到 router。
func (h *Handler) RegisterAll(r *network.Router) {
	// 所有行动阶段消息（含角色选择）都走同一个路由逻辑
	inGameMsgs := []uint16{
		protocol.MsgSelectCharacterReq,
		protocol.MsgPlayCardReq,
		protocol.MsgMoveToSynthReq,
		protocol.MsgSynthesizeReq,
		protocol.MsgUseSkillReq,
		protocol.MsgTriggerLibrateReq,
		protocol.MsgEndActionReq,
	}
	for _, msgID := range inGameMsgs {
		id := msgID // 闭包捕获副本，避免循环变量问题（Go 1.22 已修复，显式保留更清晰）
		r.Register(id, func(s *network.Session, data []byte) {
			h.routeToEngine(s, id, data)
		})
	}
}

// OnRoomCreated 由 RoomManager 的 OnRoomCreated 回调触发，为新房间创建并启动 Engine。
// 在新 goroutine 中调用，不阻塞 CreateRoom 流程。
func (h *Handler) OnRoomCreated(r *room.Room) {
	eng := NewEngine(r)
	h.engines.Store(r.ID, eng)
	slog.Info("engine created", "roomID", r.ID)
	eng.Start()
}

// routeToEngine 找到该 Session 对应的 Engine，投递行动消息。
func (h *Handler) routeToEngine(s *network.Session, msgID uint16, data []byte) {
	// ① Session → Player
	p := h.playerMgr.GetBySession(s.ID)
	if p == nil {
		slog.Warn("action from unregistered session", "sessionID", s.ID)
		return
	}

	// ② Player → Room
	roomID := p.RoomID()
	if roomID == "" {
		// 玩家不在任何房间（可能发错消息），静默丢弃
		return
	}

	// ③ Room → Engine
	v, ok := h.engines.Load(roomID)
	if !ok {
		slog.Warn("no engine for room", "roomID", roomID)
		return
	}
	eng := v.(*Engine)

	// ④ 找到座位，投递行动
	r := h.roomMgr.Get(roomID)
	if r == nil {
		return
	}
	seat := r.SeatOf(p.ID)
	if seat < 0 {
		return
	}

	eng.SubmitAction(seat, msgID, data)
}

// StopEngine 停止并移除指定房间的引擎（游戏结束时由外部调用）。
// 目前由 Engine 自身在游戏结束后调用 Stop()，此方法供未来扩展使用。
func (h *Handler) StopEngine(roomID string) {
	v, ok := h.engines.LoadAndDelete(roomID)
	if ok {
		v.(*Engine).Stop()
	}
}
