package card

import (
	"errors"
	"fmt"
)

// ════════════════════════════════════════════════════════════════
//  合成选项（受场地效果影响）
// ════════════════════════════════════════════════════════════════

// ReincarnationRule 枚举轮回牌参与合成时的特殊计算规则。
// 由场地效果"轮回之境·实/虚"激活。
type ReincarnationRule int8

const (
	ReincarnationNormal   ReincarnationRule = iota // 标准规则（无场地效果）
	ReincarnationAsBase                            // 轮回之境·实：结果 = 轮回牌自身点数
	ReincarnationAsOther                           // 轮回之境·虚：结果 = 非轮回牌的点数
)

// SynthesisOpts 是合成操作的配置，由当前生效的场地效果决定。
//
// 为什么用 Options 结构体而不是全局变量？
//   场地效果只在本阶段有效，传入 opts 让合成函数保持"纯函数"特性：
//   相同输入 + 相同 opts = 相同输出，便于测试和调试。
type SynthesisOpts struct {
	// PointsCap 是合成结果的点数上限。
	// 默认 5，IllusionBonus 激活且结果为虚幻牌时可提升至 7。
	PointsCap int

	// ReincarnationRule 控制轮回牌参与合成时的计算方式。
	ReincarnationRule ReincarnationRule

	// IllusionBonus：虚幻之境·实 场地效果——结果牌为虚幻子系时，点数上限提升至 7。
	// 若结果不是虚幻牌，仍使用 PointsCap（默认 5）。
	IllusionBonus bool

	// AllowSameType：混沌之域 场地效果——允许同功能牌型（如攻击+攻击）合成。
	AllowSameType bool
}

// DefaultOpts 返回无场地效果时的标准合成选项。
func DefaultOpts() SynthesisOpts {
	return SynthesisOpts{
		PointsCap:         MaxPoints,
		ReincarnationRule: ReincarnationNormal,
	}
}

// ════════════════════════════════════════════════════════════════
//  合成验证
// ════════════════════════════════════════════════════════════════

// ErrSameCardType 是同种牌型合成的错误，对应协议层 ErrCodeSynthSameType。
var ErrSameCardType = errors.New("同种牌型无法合成")

// Validate 检查两张牌能否合成。
// 游戏规则约束：同种功能牌型（攻击+攻击、技能+技能、能耗+能耗）均禁止。
// 大系相同还是不同不影响合法性，只影响点数计算方式。
func Validate(a, b *Card) error {
	if a == nil || b == nil {
		return errors.New("不能合成空牌")
	}
	if a.CardType == b.CardType {
		return fmt.Errorf("%w（%s + %s）", ErrSameCardType, a.CardType, b.CardType)
	}
	return nil
}

// ════════════════════════════════════════════════════════════════
//  合成算法
// ════════════════════════════════════════════════════════════════

// Combine 将两张牌合成为一张新牌。
//
// 结果牌的属性：
//   - SubFaction、CardType 继承自 base（第一张牌）
//   - Points 由合成规则计算，受 opts 影响后截断到 PointsCap
//   - IsHidden = false（合成产生的牌点数公开，隐藏状态来自场地效果，在抽牌时设置）
//
// 为什么结果继承 base 的属性？
//   玩家选择"哪张牌作为 base"就是在选择结果的功能类型。
//   这给了玩家主动权：想要攻击牌结果，就把攻击牌放在第一位置。
//   比"随机决定"或"固定规则决定"更有策略性。
func Combine(base, ingredient *Card, opts SynthesisOpts) (*Card, error) {
	// 混沌之域：跳过同类型检查；否则走标准验证
	if !opts.AllowSameType {
		if err := Validate(base, ingredient); err != nil {
			return nil, err
		}
	} else if base == nil || ingredient == nil {
		return nil, errors.New("不能合成空牌")
	}

	points := calcPoints(base, ingredient, opts)

	// 点数上限截断
	// 虚幻之境·实：结果为虚幻牌时上限 7，否则沿用 PointsCap（默认 5）
	cap := opts.PointsCap
	if cap <= 0 {
		cap = MaxPoints
	}
	if opts.IllusionBonus && base.SubFaction == SubIllusion {
		cap = MaxPointsWithField
	}
	if points > cap {
		points = cap
	}

	return &Card{
		ID:         newCardID(),
		SubFaction: base.SubFaction,
		CardType:   base.CardType,
		Points:     points,
	}, nil
}

// calcPoints 计算合成点数，不含上限截断。
func calcPoints(base, ingredient *Card, opts SynthesisOpts) int {
	// ── 场地效果：轮回之境·实 ─────────────────────────────────
	// 只要有一张是轮回牌，结果等于轮回牌自身的点数。
	if opts.ReincarnationRule == ReincarnationAsBase {
		if base.SubFaction == SubReincarnation {
			return base.Points
		}
		if ingredient.SubFaction == SubReincarnation {
			return ingredient.Points
		}
	}

	// ── 场地效果：轮回之境·虚 ─────────────────────────────────
	// 只要有一张是轮回牌，结果等于非轮回牌的点数。
	if opts.ReincarnationRule == ReincarnationAsOther {
		if base.SubFaction == SubReincarnation {
			return ingredient.Points
		}
		if ingredient.SubFaction == SubReincarnation {
			return base.Points
		}
	}

	// ── 标准合成规则 ───────────────────────────────────────────
	if base.SubFaction.Major() == ingredient.SubFaction.Major() {
		// 同大系（梦幻+梦幻 or 重回+重回）→ 点数相乘
		return base.Points * ingredient.Points
	}
	// 不同大系（梦幻+重回）→ 点数相加
	return base.Points + ingredient.Points
}
