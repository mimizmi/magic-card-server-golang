package protocol

// ════════════════════════════════════════════════════════════════
//  视图层：信息遮蔽（Information Hiding）
//
//  核心原则：服务端永远持有完整游戏状态（GameSnapshot），
//  但发给客户端的永远是经过过滤的"视图"（GameStateView）。
//
//  游戏里每位玩家只能看到：
//    1. 自己的全部信息（手牌、合成区、角色、HP、能量）
//    2. 对手的公开信息（HP、能量、手牌数量、是否濒死）
//    3. 对手的角色：暗置时显示 "???"，使用技能后才公开
//    4. 对手手牌内容：永远不发给你（只告诉你对手有几张）
//    5. 对手合成区：只告诉你有几张，内容隐藏
//
//  特殊例外（通过单独的事件消息处理，不在此文件）：
//    - 镜换者一级技能：本阶段内可见对手最高攻击牌点数（MsgSkillUsedEv 附带）
//    - 虚幻之境·虚场地效果：对手的非虚幻牌点数本阶段隐藏
// ════════════════════════════════════════════════════════════════

// PendingAttackView 告知客户端当前行动阶段有一次待防御的攻击。
// 只在攻击已打出、防御窗口开启时存在，其他时候为 nil（JSON omitempty）。
type PendingAttackView struct {
	AttackerSeat int `json:"attacker_seat"`
	AttackPoints int `json:"attack_points"`
}

// GameStateView 是发送给某一位玩家的游戏状态视图。
// Phase 4（游戏引擎）会调用 BuildView 填充这个结构体。
type GameStateView struct {
	Round         int                `json:"round"`
	Phase         string             `json:"phase"`         // "field_draw","draw","action","combat","cleanup","secret_realm"
	ActiveSeat    int                `json:"active_seat"`   // 当前该谁操作
	FieldEffect   string             `json:"field_effect"`  // 当前场地效果名称
	PendingAttack *PendingAttackView `json:"pending_attack,omitempty"` // 非nil=防御窗口开启
	Me            PlayerView         `json:"me"`
	Opponent      OpponentView       `json:"opponent"`
}

// PlayerView 是玩家看到的自己的完整信息。
type PlayerView struct {
	Seat        int        `json:"seat"`
	HP          int        `json:"hp"`
	MaxHP       int        `json:"max_hp"`
	Energy      int        `json:"energy"`
	MaxEnergy   int        `json:"max_energy"`
	Character   string     `json:"character"`    // 角色名，未公开时为 "???"（但自己永远知道自己的角色）
	IsNearDeath bool       `json:"is_near_death"`
	Hand        []CardView     `json:"hand"`                  // 手牌区，最多 8 张
	SynthZone   []CardView     `json:"synth_zone"`            // 合成区，最多 4 张
	// ExtraInfo 存放角色特定的额外状态（如时空裂缝者的裂缝数量和产能）
	// 不需要时为 nil，JSON 序列化会省略此字段
	ExtraInfo   map[string]any `json:"extra_info,omitempty"`
}

// OpponentView 是玩家看到的对手的受限信息。
// 注意没有 Hand 字段——对手的手牌内容不发给你。
type OpponentView struct {
	Seat        int    `json:"seat"`
	HP          int    `json:"hp"`
	MaxHP       int    `json:"max_hp"`
	Energy      int    `json:"energy"`    // 能量条数值双方可见（游戏设计上是公开信息）
	MaxEnergy   int    `json:"max_energy"`
	Character   string `json:"character"` // 暗置时 "???"，使用过技能后公开
	IsNearDeath bool   `json:"is_near_death"`
	HandCount   int    `json:"hand_count"`  // 对手手牌张数（数量公开，内容不公开）
	SynthCount  int    `json:"synth_count"` // 对手合成区张数
}

// CardView 是一张牌对某位玩家呈现的样子。
//
// Points 使用指针类型 *int，nil 表示"点数被隐藏"。
// 这比用 -1 或 0 表示隐藏更安全：类型系统强制调用方检查 nil，
// 不会因为忘记检查而把 -1 当成真实点数用于计算。
//
// 什么时候 Points 为 nil？
//   - 虚幻之境·虚 场地效果下，自己手中的非虚幻牌（对手视角的隐藏）
//   - 虚幻之境·实 场地效果下，新抽到的虚幻牌（结算时才揭示，+2 后最高 7 点）
type CardView struct {
	Slot     int    `json:"slot"`
	Faction  string `json:"faction"`   // "梦幻" or "重回"
	CardType string `json:"card_type"` // "攻击" or "技能" or "能耗"
	Points   *int   `json:"points"`    // nil = 点数隐藏
}

// ════════════════════════════════════════════════════════════════
//  BuildView — 信息遮蔽的核心函数
//
//  这个函数在 Phase 4（游戏引擎）完成后会被实装。
//  现在先定义好接口和注释，Phase 4 填入真实游戏状态。
//
//  参数说明：
//    snap     - 服务端完整游戏快照（Phase 4 定义）
//    forSeat  - 为哪位玩家（0 或 1）生成视图
//
//  调用方式（Phase 4 之后）：
//    view := protocol.BuildView(gameSnap, 0)  // 给 0 号玩家的视图
//    data := protocol.MustEncode(view)
//    session.Send(protocol.MsgGameStateEv, data)
// ════════════════════════════════════════════════════════════════

// intPtr 是辅助函数，将 int 转换为 *int。
// 用于构造 CardView.Points 字段（有值时），让代码更简洁。
//
// 示例：
//
//	card := CardView{Points: intPtr(3)}  // 点数为 3，非隐藏
//	card := CardView{Points: nil}        // 点数隐藏
func IntPtr(v int) *int {
	return &v
}
