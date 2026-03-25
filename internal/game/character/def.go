package character

import "fmt"

// ════════════════════════════════════════════════════════════════
//  角色系统核心类型定义
// ════════════════════════════════════════════════════════════════

// SkillTier 区分技能的三个档位。
type SkillTier int

const (
	TierNormal    SkillTier = 1 // 普通技能（技能牌点数 1-2）
	TierEnhanced  SkillTier = 2 // 强化技能（技能牌点数 3-5）
	TierLiberation SkillTier = 3 // 解放技能（TriggerLibrateReq / 殉道者自动）
)

// SkillResult 描述一次技能激活产生的效果。
//
// 设计原则：纯数据——Engine 接收此结构并决定如何应用到 GameState。
// 角色包不知道 GameState 的存在，零循环依赖。
type SkillResult struct {
	Tier             SkillTier
	DealDirectDamage int    // 对对手造成 N 点直接伤害（不依赖攻击牌）
	HealSelf         int    // 为施法者恢复 N 点 HP
	GainEnergy       int    // 为施法者增加 N 点能量
	DrawCards        int    // 施法者抽 N 张牌
	Desc             string // 面向客户端展示的效果描述（写入 SkillUsedEv.Desc）
}

// PassiveTraits 描述角色始终生效的被动特性。
// 零值表示该被动无效（无需 nil 判断）。
type PassiveTraits struct {
	// BonusOutgoing 加到该玩家每次攻击牌的伤害上。
	BonusOutgoing int

	// IncomingReduction 从该玩家每次受到的伤害中扣除。
	// 最终伤害最低为 1，不会归零。
	IncomingReduction int

	// InterceptNearDeath：殉道者被动——在二次死亡时自动触发解放，而非直接死亡。
	// 每局仅生效一次（CharInstance.InterceptUsed 跟踪是否已用）。
	InterceptNearDeath bool
}

// SkillDef 定义一个技能档位的静态数据。
type SkillDef struct {
	Name       string
	EnergyCost int
	Result     SkillResult // 激活此技能产生的效果
}

// CharDef 是角色的静态定义（不可变，存于 Registry）。
type CharDef struct {
	ID           string
	Name         string

	// 初始属性——选择此角色时覆盖 PlayerState 的默认值
	MaxHP        int
	MaxEnergy    int
	LibThreshold int // 触发解放技能所需的最低能量

	// ManualLib: true = 玩家手动发送 TriggerLibrateReq
	//            false = 引擎自动触发（殉道者在二次死亡时）
	ManualLib bool

	Passive  PassiveTraits
	Normal   SkillDef // 技能牌点数 1-2 触发
	Enhanced SkillDef // 技能牌点数 3-5 触发
	Lib      SkillDef // 解放技能
}

// ════════════════════════════════════════════════════════════════
//  注册表
// ════════════════════════════════════════════════════════════════

var registry = map[string]*CharDef{}

// register 在包初始化时注册一个角色定义（由 chars.go 的 init() 调用）。
func register(c *CharDef) {
	registry[c.ID] = c
}

// Get 按 ID 查找角色定义，找不到返回 false。
func Get(id string) (*CharDef, bool) {
	c, ok := registry[id]
	return c, ok
}

// All 返回所有已注册角色的定义切片（顺序不固定）。
func All() []*CharDef {
	result := make([]*CharDef, 0, len(registry))
	for _, c := range registry {
		result = append(result, c)
	}
	return result
}

// MustGet 查找角色，不存在时 panic。
// 仅用于测试和内部初始化，运行时应使用 Get。
func MustGet(id string) *CharDef {
	c, ok := Get(id)
	if !ok {
		panic(fmt.Sprintf("character not found: %s", id))
	}
	return c
}
