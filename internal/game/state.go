package game

import (
	"echo/internal/game/card"
	"echo/internal/game/character"
	"echo/internal/game/field"
)

// ════════════════════════════════════════════════════════════════
//  Phase — 游戏阶段
// ════════════════════════════════════════════════════════════════

// Phase 使用 string 常量而非 iota，原因：
//   - 序列化到 JSON 时直接可读（"action" 比 3 有意义得多）
//   - 协议层 PhaseChangeEv.Phase 是 string 类型，无需转换
type Phase string

const (
	PhaseWaiting   Phase = "waiting"    // 等待双方选择角色
	PhaseFieldDraw Phase = "field_draw" // 场地/道具抽取（第2回合起）
	PhaseDraw      Phase = "draw"       // 补牌
	PhaseAction    Phase = "action"     // 行动（双方轮流出牌）
	PhaseCombat    Phase = "combat"     // 交战结算
	PhaseCleanup   Phase = "cleanup"    // 清场
	PhaseGameOver  Phase = "game_over"  // 游戏结束
)

// ════════════════════════════════════════════════════════════════
//  PlayerState — 单个玩家的游戏内状态
// ════════════════════════════════════════════════════════════════

// PlayerState 持有一名玩家在一局游戏中的所有可变状态。
//
// 注意：这是服务端的"真相"，永远不直接发给客户端。
// 发给客户端的是经过 BuildView 过滤的 protocol.PlayerView / OpponentView。
type PlayerState struct {
	Seat      int
	HP        int
	MaxHP     int
	Energy    int
	MaxEnergy int

	// LibThreshold 是该角色的解放阈值，能量达到此值时可触发解放技能。
	// Phase 7 中由具体角色实现覆盖，现阶段默认 80。
	LibThreshold int

	// IsNearDeath：HP 首次归零后进入濒死状态（HP 回到 60，每阶段扣 30）
	IsNearDeath bool
	// BlessingUsed：赐福已触发（HP < 40，第二角色已激活）
	BlessingUsed bool

	Hand *card.HandZone
	Deck *card.Deck

	// PlayedAttack 是本轮行动阶段玩家"提交"的攻击牌，
	// 交战阶段用此牌的点数造成伤害。行动阶段可替换（后出覆盖前出）。
	PlayedAttack *card.Card

	// ActionDone：玩家是否已宣告本轮行动结束
	ActionDone bool

	// ── 角色系统 ────────────────────────────────────────────
	// CharacterID 保留供 view.go 快速读取，与 Char.Def.ID 保持同步。
	CharacterID  string // 角色标识符，如 "licai"（力裁者）
	CharRevealed bool   // 使用过技能后角色公开（对手可见）

	// Char：运行时角色实例，选角后创建，nil 表示尚未选角。
	Char *character.CharInstance

	// SecondChar：赐福激活的第二角色（HP < 40 时随机分配，Phase 8 实装）
	SecondChar *character.CharInstance
}

// newPlayerState 创建初始状态的玩家。
func newPlayerState(seat int) *PlayerState {
	return &PlayerState{
		Seat:         seat,
		HP:           100,
		MaxHP:        100,
		Energy:       10,
		MaxEnergy:    100,
		LibThreshold: 80, // 默认值，Phase 7 角色会覆盖
		Hand:         card.NewHandZone(),
		Deck:         card.NewDeck(),
	}
}

// drawCards 补充手牌至 n 张（正常 8，濒死 4）。
func (p *PlayerState) drawCards() {
	maxSlots := card.HandZoneSize
	if p.IsNearDeath {
		maxSlots = card.SafeZoneSize // 濒死只能补安全区
	}
	p.Hand.Fill(p.Deck, maxSlots)
}

// ════════════════════════════════════════════════════════════════
//  GameState — 一局游戏的完整快照
// ════════════════════════════════════════════════════════════════

// GameState 是服务端维护的权威游戏状态。
//
// 设计原则：Engine 是这个结构体的唯一写者，其他所有 goroutine
// 只能通过 Engine.SubmitAction 投递消息，不直接修改 GameState。
// 这保证了"单写者"的线程安全，无需对 GameState 加锁。
type GameState struct {
	GameID string
	Round  int
	Phase  Phase

	// ActiveSeat 是行动阶段当前该谁操作（0 或 1）。
	// 非行动阶段时值无意义。
	ActiveSeat int

	Players [2]*PlayerState

	// FieldEffect 是本回合生效的场地效果（nil = 无效果，仅第1回合）
	FieldEffect *field.FieldEffect

	// Winner：-1 = 游戏未结束，0 or 1 = 获胜方座位
	Winner int
}

// newGameState 创建初始游戏状态。
func newGameState(gameID string) *GameState {
	return &GameState{
		GameID:  gameID,
		Round:   0,
		Phase:   PhaseWaiting,
		Players: [2]*PlayerState{newPlayerState(0), newPlayerState(1)},
		Winner:  -1,
	}
}

// isOver 返回游戏是否已结束。
func (gs *GameState) isOver() bool {
	return gs.Winner >= 0
}
