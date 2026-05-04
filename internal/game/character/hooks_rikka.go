package character

import "fmt"

func init() {
	// 节律者：手牌上限随能量动态浮动；技能必须按"递增点数"顺序使用，回合开始重置。
	// 被动：技能内抽牌时手牌上限 = 当前能量值；每回合开始的固定补 8 张不受此影响。
	// 技能：使用 n 点的技能牌时，先消耗 n 点能量，再对敌方造成 n 点直接伤害并抽 n 张牌；
	//       此时手牌上限 = 扣能量后的能量值，若抽 n 张会超过上限，则仅补到上限数量为止。
	// 限制：本回合每次出技能牌的点数必须 > 上一次出的技能牌点数（行动阶段开始重置）。
	//
	// ExtraState：
	//   last_skill_pts int —— 本回合最近一次成功使用技能牌的点数
	registry["rikka"] = &CharDef{
		ID: "rikka",
		Hooks: &CharHooks{
			// 行动阶段开始：清空"上一次技能点数"，使本回合首张技能牌不受限制。
			OnPhaseStart: func(phase string, es map[string]any) (int, string) {
				if phase == "action" {
					es["last_skill_pts"] = 0
				}
				return 0, ""
			},

			// 手牌上限 = 能量值。返回 0 让引擎沿用默认（HandZoneSize / 濒死 SafeZoneSize）。
			MaxHandSize: func(_ map[string]any, energy int) int {
				if energy <= 0 {
					return 1 // 至少保留 1 张，避免完全摸不到牌而锁死局面
				}
				return energy
			},

			// 技能牌前置校验：本回合点数必须 > 上一次。
			PreUseSkillCheck: func(pts int, es map[string]any) error {
				last := esInt(es, "last_skill_pts", 0)
				if pts <= last {
					return fmt.Errorf("节律：本回合技能点数必须高于上次（上次 %d 点）", last)
				}
				return nil
			},

			// 替换默认技能档位：直接造成 pts 伤害 + 抽 pts 张牌，消耗 1 点能量。
			UseSkillOverride: func(pts int, es map[string]any) (*SkillResult, int, bool) {
				cost := pts // 技能消耗 = 牌面点数
				if pts <= 0 {
					return nil, 0, false // 不应到此处；走默认逻辑兜底
				}
				es["last_skill_pts"] = pts
				return &SkillResult{
					Tier:             TierEnhanced,
					DealDirectDamage: pts,
					DrawCards:        pts,
					Desc:             fmt.Sprintf("节律：造成 %d 点直接伤害并抽 %d 张牌", pts, pts),
				}, cost, true
			},

			BuildExtraInfo: func(es map[string]any) map[string]any {
				return map[string]any{
					"jielv_last_skill_pts": esInt(es, "last_skill_pts", 0),
				}
			},
		},
	}
}
