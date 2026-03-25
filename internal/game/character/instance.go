package character

import "errors"

// ════════════════════════════════════════════════════════════════
//  CharInstance — 角色的运行时状态
// ════════════════════════════════════════════════════════════════

// CharInstance 持有一名玩家已选角色在一局游戏中的可变状态。
//
// 不可变的定义（技能效果、初始属性）放在 CharDef，
// 只有"已用过"类的状态放在此结构。
type CharInstance struct {
	Def *CharDef

	// LibUsed：解放技能是否已触发过（每局只能触发一次）
	LibUsed bool

	// InterceptUsed：殉道者被动"二次死亡拦截"是否已用过
	InterceptUsed bool
}

// NewInstance 根据角色 ID 创建运行时实例。
// 角色 ID 未注册时返回错误。
func NewInstance(charID string) (*CharInstance, error) {
	def, ok := Get(charID)
	if !ok {
		return nil, errors.New("未知角色 ID: " + charID)
	}
	return &CharInstance{Def: def}, nil
}

// ════════════════════════════════════════════════════════════════
//  技能激活
// ════════════════════════════════════════════════════════════════

// UseSkill 根据技能牌点数决定激活普通技能还是强化技能。
// 返回：(技能结果, 能量消耗, 错误)
//
// 判定规则：
//   cardPoints ≤ 2 → TierNormal
//   cardPoints ≥ 3 → TierEnhanced
func (ci *CharInstance) UseSkill(cardPoints int) (*SkillResult, int, error) {
	var skill SkillDef
	if cardPoints <= 2 {
		skill = ci.Def.Normal
	} else {
		skill = ci.Def.Enhanced
	}
	result := skill.Result // 值拷贝，防止外部意外修改定义
	return &result, skill.EnergyCost, nil
}

// TriggerLiberation 触发解放技能。
// 若已触发过，返回错误；触发后标记 LibUsed=true。
// 能量消耗由调用方（engine）在调用前检查并扣除。
func (ci *CharInstance) TriggerLiberation() (*SkillResult, error) {
	if ci.LibUsed {
		return nil, errors.New("解放技能每局只能使用一次")
	}
	ci.LibUsed = true
	result := ci.Def.Lib.Result // 值拷贝
	return &result, nil
}

// CanLiberate 检查是否可以手动触发解放（未用过 + 能量足够）。
func (ci *CharInstance) CanLiberate(energy int) bool {
	return !ci.LibUsed && energy >= ci.Def.LibThreshold
}

// ════════════════════════════════════════════════════════════════
//  被动钩子
// ════════════════════════════════════════════════════════════════

// ModifyOutgoing 将被动"攻击加成"应用到攻击伤害上。
// engine 在每次结算攻击牌伤害时调用此方法。
func (ci *CharInstance) ModifyOutgoing(damage int) int {
	return damage + ci.Def.Passive.BonusOutgoing
}

// ModifyIncoming 将被动"伤害减免"应用到受到的伤害上。
// 最终伤害最低为 1（不能被减免到 0）。
func (ci *CharInstance) ModifyIncoming(damage int) int {
	d := damage - ci.Def.Passive.IncomingReduction
	if d < 1 {
		d = 1
	}
	return d
}

// InterceptSecondDeath 尝试用被动拦截二次死亡（殉道者）。
// 返回 true 表示成功拦截，此后 LibUsed 和 InterceptUsed 均设为 true。
// 每局只能拦截一次。
func (ci *CharInstance) InterceptSecondDeath() bool {
	if !ci.Def.Passive.InterceptNearDeath {
		return false
	}
	if ci.InterceptUsed {
		return false
	}
	ci.InterceptUsed = true
	ci.LibUsed = true // 自动触发解放，标记解放已用
	return true
}
