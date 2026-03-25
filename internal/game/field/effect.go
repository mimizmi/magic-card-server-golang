package field

import "math/rand"

// ════════════════════════════════════════════════════════════════
//  场地效果系统
// ════════════════════════════════════════════════════════════════

// EffectID 是场地效果的唯一标识符（字符串常量，便于日志和协议）。
type EffectID string

const (
	EffectClear        EffectID = "clear"        // 空旷之地：无效果
	EffectIllusionReal EffectID = "illusion_real" // 虚幻之境·实：虚幻牌合成上限提升至 7
	EffectIllusionVoid EffectID = "illusion_void" // 虚幻之境·虚：本回合补入的牌点数对对手隐藏
	EffectReincBase    EffectID = "reinc_base"   // 轮回之境·实：轮回牌参与合成时结果 = 轮回牌自身点数
	EffectReincOther   EffectID = "reinc_other"  // 轮回之境·虚：轮回牌参与合成时结果 = 另一张牌的点数
	EffectChaos        EffectID = "chaos"        // 混沌之域：允许同功能牌型合成
	EffectEcho         EffectID = "echo"         // 回响之地：攻击牌伤害 +1
	EffectProtect      EffectID = "protect"      // 守护之光：濒死玩家每轮扣血从 30 减少至 15
)

// ReincarnHint 镜像 card.ReincarnationRule，避免循环导入。
// engine.go 负责将此值转换为 card.SynthesisOpts.ReincarnationRule。
type ReincarnHint int8

const (
	ReincNormal  ReincarnHint = 0 // 标准规则
	ReincAsBase  ReincarnHint = 1 // 轮回之境·实
	ReincAsOther ReincarnHint = 2 // 轮回之境·虚
)

// FieldEffect 描述一种场地效果对游戏规则的所有影响。
//
// 设计原则：
//   - 纯数据结构，不包含任何行为方法（行为由 engine.go 解释执行）
//   - 每个字段对应一个明确的规则修改点，零值 = 无影响
type FieldEffect struct {
	ID   EffectID
	Name string // 显示名称，直接发给客户端

	// ── 合成相关 ─────────────────────────────────────────────────
	// IllusionBonus：虚幻牌合成上限提升至 7（card.MaxPointsWithField）
	IllusionBonus bool
	// AllowSameType：允许同功能牌型合成（跳过 ErrSameCardType 检查）
	AllowSameType bool
	// ReincarnRule：控制轮回牌的合成点数计算方式
	ReincarnRule ReincarnHint

	// ── 手牌相关 ─────────────────────────────────────────────────
	// HideDrawnCards：本回合补牌后，新牌的 IsHidden = true
	// 对手视图中该牌点数将显示为 nil（信息遮蔽）
	HideDrawnCards bool

	// ── 战斗相关 ─────────────────────────────────────────────────
	// BonusAttack：每次攻击额外附加的固定伤害
	BonusAttack int

	// ── 濒死相关 ─────────────────────────────────────────────────
	// NearDeathDrain：濒死玩家每轮清场扣血量（0 = 使用默认值 30）
	NearDeathDrain int
}

// ActualNearDeathDrain 返回本效果下的濒死扣血量。
// 若未设置（0），返回游戏默认值 30。
func (e *FieldEffect) ActualNearDeathDrain() int {
	if e.NearDeathDrain > 0 {
		return e.NearDeathDrain
	}
	return 30
}

// ════════════════════════════════════════════════════════════════
//  场地效果池
// ════════════════════════════════════════════════════════════════

// Pool 是所有可能出现的场地效果，每局游戏从此池中随机抽取。
// 8 种效果保持等概率，不做权重调整（Phase 8 可加权）。
var Pool = []*FieldEffect{
	{
		ID:   EffectClear,
		Name: "空旷之地",
		// 无任何修改——提供"无效果"回合，给玩家喘息机会
	},
	{
		ID:            EffectIllusionReal,
		Name:          "虚幻之境·实",
		IllusionBonus: true,
		// 虚幻子系牌合成上限从 5 突破至 7，鼓励虚幻牌合成策略
	},
	{
		ID:             EffectIllusionVoid,
		Name:           "虚幻之境·虚",
		HideDrawnCards: true,
		// 本回合补入的牌对对手隐藏，增加信息不对称
	},
	{
		ID:           EffectReincBase,
		Name:         "轮回之境·实",
		ReincarnRule: ReincAsBase,
		// 轮回牌参与合成时，结果点数 = 轮回牌自身点数（稳定输出）
	},
	{
		ID:           EffectReincOther,
		Name:         "轮回之境·虚",
		ReincarnRule: ReincAsOther,
		// 轮回牌参与合成时，结果点数 = 另一张牌点数（牺牲轮回牌保留材料）
	},
	{
		ID:            EffectChaos,
		Name:          "混沌之域",
		AllowSameType: true,
		// 解除同功能牌型不可合成的限制，大幅扩展合成选择
	},
	{
		ID:          EffectEcho,
		Name:        "回响之地",
		BonusAttack: 1,
		// 所有攻击牌额外 +1 伤害，提高本轮攻防节奏
	},
	{
		ID:             EffectProtect,
		Name:           "守护之光",
		NearDeathDrain: 15,
		// 濒死扣血从 30 减至 15，给处于濒死的玩家更多翻盘机会
	},
}

// Draw 从效果池中随机抽取一种场地效果。
// 使用传入的 *rand.Rand 而非全局随机源，保证游戏房间间的随机独立性。
func Draw(r *rand.Rand) *FieldEffect {
	return Pool[r.Intn(len(Pool))]
}
