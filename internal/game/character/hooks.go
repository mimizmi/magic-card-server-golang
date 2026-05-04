package character

// CharHooks 包含角色的可选行为钩子，允许特殊角色覆盖或扩展引擎的标准逻辑。
// 所有函数字段均为可选（nil = 使用默认行为），引擎在调用前检查 nil。
type CharHooks struct {

	// ── 属性标志 ─────────────────────────────────────────────────

	// HPEnergyShared：HP 和能量共享同一数值。
	// 能量增加时 HP 同步增加，受到伤害时能量同步减少。
	HPEnergyShared bool

	// AllCardsAsAttack：所有手牌均视为攻击牌出牌，优先级高于场地效果。
	AllCardsAsAttack bool

	// LibRepeatable：解放技能可多次触发（不受 LibUsed 限制）。
	LibRepeatable bool

	// InitHP / InitEnergy：覆盖选角后的初始值（0 = 使用 MaxHP / MaxEnergy）。
	InitHP     int
	InitEnergy int

	// ── 阶段钩子 ─────────────────────────────────────────────────

	// OnPhaseStart 在每个阶段开始时对角色拥有者调用。
	// 返回能量变化量（正=获得，负=消耗）和可选日志文本。
	// 若 HPEnergyShared，引擎同步修改 HP。
	OnPhaseStart func(phase string, es map[string]any) (energyDelta int, msg string)

	// ── 伤害钩子 ─────────────────────────────────────────────────

	// ModifyIncomingDamage 在被动减免之后、HP扣除之前调用。
	// 返回 (实际受到的伤害, 反弹给来源的伤害)。0=完全免疫。
	// damageType: "attack"/"skill direct"/"skill self-damage"/"cleanup" 等。
	ModifyIncomingDamage func(damage int, damageType string, es map[string]any) (int, int)

	// OnLethalCheck 在 HP 归零触发死亡判定之前调用。
	// opponentHP 为对手当前 HP，允许角色根据对手状态决定是否触发保命。
	// 返回 (是否存活, 存活后HP, 反弹给对手的伤害)。
	OnLethalCheck func(damage int, es map[string]any, opponentHP int) (survive bool, hpAfter int, reflectDmg int)

	// OnDamageReceived 在此玩家受到任意来源的最终伤害后调用。
	OnDamageReceived func(finalDamage int, es map[string]any)

	// OnDamageDealt 在此玩家对对手造成伤害后调用（不含自伤）。
	OnDamageDealt func(finalDamage int, es map[string]any)

	// OnDamageLanded 在此玩家对对手造成伤害后调用，返回自身回复量（0=不回复）。
	// 用于吸血效果。
	OnDamageLanded func(finalDamage int, es map[string]any) int

	// OnAttackHit 在此玩家通过攻击牌成功造成 >0 最终伤害后调用，返回需补抽的牌数（0=不补抽）。
	// 仅由攻击路径触发；不会被技能直接伤害、反弹伤害或濒死扣血触发。
	OnAttackHit func(finalDamage int, es map[string]any) int

	// ── 出牌钩子 ─────────────────────────────────────────────────

	// OnCardPlayed 在 handlePlayCard 取出牌后、AllCardsAsAttack 转换前立刻调用。
	// 角色可读取卡牌的"原始功能"（攻击/技能/能耗）以做累积分类等统计。
	OnCardPlayed func(cardType string, points int, faction string, es map[string]any)

	// IsAttackUndefendable 在攻击牌即将创建 PendingAttack 之前调用。
	// 返回 true 时引擎跳过防御窗口，直接对对手施加该次攻击伤害（视为技能伤害）。
	IsAttackUndefendable func(es map[string]any) bool

	// ── 攻击修正钩子 ─────────────────────────────────────────────

	// ModifyCardPoints 在 BonusOutgoing 和场地加成之前修改牌面点数。
	ModifyCardPoints func(pts int, es map[string]any) int

	// ModifyOutgoingAttack 在 BonusOutgoing 和场地加成之后对最终攻击点数进行修正。
	// 应返回修正后的最终值（而非增量）。
	ModifyOutgoingAttack func(pts int, energy int, es map[string]any) int

	// OnAttackLaunched 在创建 PendingAttack 之前调用。
	// 返回 (额外攻击点数, 消耗能量)，引擎将额外点数加入攻击并扣除能量。
	OnAttackLaunched func(attackPoints int, energy int, es map[string]any) (extraPoints int, energySpent int)

	// ── 客户端展示 ─────────────────────────────────────────────────

	// BuildExtraInfo 返回要发送给客户端的额外状态数据（如裂缝数、房子数等）。
	// 引擎在构建 PlayerView 时调用，返回 nil 表示无额外信息。
	BuildExtraInfo func(es map[string]any) map[string]any

	// BuildPublicExtra 返回对手也能看到的公开状态（护盾层数、房子数等）。
	// 引擎在构建 OpponentView 时调用，返回 nil 表示无公开信息。
	BuildPublicExtra func(es map[string]any) map[string]any

	// ── 技能覆盖 ─────────────────────────────────────────────────

	// UseSkillOverride 替换默认的技能档位判定逻辑（若设置）。
	// 返回 (result, cost, handled)。handled=false 则回退到普通/强化默认逻辑。
	UseSkillOverride func(cardPoints int, es map[string]any) (result *SkillResult, cost int, handled bool)

	// PreUseSkillCheck 在技能牌取出后、UseSkill 调用前进行前置校验。
	// 返回非 nil error 时引擎拒绝这次出牌：将牌放回原槽位，并把错误消息发给玩家。
	// 用途：节律者要求本回合每次技能点数不得低于上一次。
	PreUseSkillCheck func(cardPoints int, es map[string]any) error

	// MaxHandSize 在补牌阶段（Fill）调用，返回该角色当前的手牌上限。
	// 返回值会被引擎 clamp 到 [1, HandZoneSize]；返回 0 表示沿用引擎默认（HandZoneSize 或濒死 SafeZoneSize）。
	// 引擎传入当前能量值，便于实现"手牌上限=能量"等动态被动。
	MaxHandSize func(es map[string]any, energy int) int
}

// esInt 从 ExtraState 读取 int 值，键不存在或类型不符时返回 defVal。
func esInt(es map[string]any, key string, defVal int) int {
	if v, ok := es[key]; ok {
		if i, ok := v.(int); ok {
			return i
		}
	}
	return defVal
}

// esBool 从 ExtraState 读取 bool 值，键不存在或类型不符时返回 defVal。
func esBool(es map[string]any, key string, defVal bool) bool {
	if v, ok := es[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return defVal
}
