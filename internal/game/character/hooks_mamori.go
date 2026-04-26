package character

import (
	"fmt"
	"strings"
)

func init() {
	// 反伤者：反弹/免疫体系角色。
	// 被动：受到致命伤害时锁血1，将累计格挡/反弹的伤害全部反弹给对手（一局一次）。
	// 普通技能（未合成技能牌）：消耗能量，获得1层反弹层数（上限5），受攻击时消耗1层并反弹。
	// 强化技能（合成技能牌）：消耗能量，免疫技能伤害若干阶段。
	// 解放技（25点技能牌）：消耗能量，免疫并反弹所有伤害若干回合。
	//
	// ExtraState:
	//   "reflect_stacks"      int  - 反弹层数(0-5)，每层可反弹一次攻击
	//   "accumulated_blocked" int  - 技能1/2累计格挡/反弹的伤害总量
	//   "skill_immune_phases" int  - 剩余技能免疫阶段数
	//   "lib_immune_phases"   int  - 剩余全免疫+反弹阶段数（每回合=5个phase）
	//   "lethal_save_used"    bool - 被动保命已使用

	registry["mamori"] = &CharDef{
		ID: "mamori",
		Hooks: &CharHooks{
			OnPhaseStart: func(phase string, es map[string]any) (int, string) {
				// 每个阶段递减免疫计数器
				var msgs []string

				if v := esInt(es, "skill_immune_phases", 0); v > 0 {
					es["skill_immune_phases"] = v - 1
					if v-1 == 0 {
						msgs = append(msgs, "技能免疫结束")
					}
				}
				if v := esInt(es, "lib_immune_phases", 0); v > 0 {
					es["lib_immune_phases"] = v - 1
					if v-1 == 0 {
						msgs = append(msgs, "全伤害免疫+反弹结束")
					}
				}

				msg := ""
				if len(msgs) > 0 {
					msg = strings.Join(msgs, "；")
				}
				return 0, msg
			},

			ModifyIncomingDamage: func(damage int, damageType string, es map[string]any) (int, int) {
				// 优先级1：解放免疫（全类型免疫+反弹）
				if esInt(es, "lib_immune_phases", 0) > 0 {
					acc := esInt(es, "accumulated_blocked", 0)
					es["accumulated_blocked"] = acc + damage
					return 0, damage // 完全免疫，全额反弹
				}

				// 优先级2：技能免疫（仅技能伤害）
				if esInt(es, "skill_immune_phases", 0) > 0 && damageType == "skill direct" {
					acc := esInt(es, "accumulated_blocked", 0)
					es["accumulated_blocked"] = acc + damage
					return 0, 0 // 免疫技能伤害，不即时反弹（累计到被动）
				}

				// 优先级3：反弹层数（仅攻击伤害）
				isAttack := strings.Contains(damageType, "攻击") || strings.Contains(damageType, "attack")
				stacks := esInt(es, "reflect_stacks", 0)
				if stacks > 0 && isAttack {
					es["reflect_stacks"] = stacks - 1 // 消耗一层
					acc := esInt(es, "accumulated_blocked", 0)
					es["accumulated_blocked"] = acc + damage
					return damage, damage // 受到伤害同时反射等量伤害
				}

				return damage, 0 // 正常受伤
			},

			OnLethalCheck: func(damage int, es map[string]any, opponentHP int) (bool, int, int) {
				if esBool(es, "lethal_save_used", false) {
					return false, 0, 0 // 已用过
				}
				if opponentHP <= 0 {
					return false, 0, 0 // 对手已死
				}

				// 被动：受到致命伤害时锁血1，将累计格挡的所有伤害反弹给对手
				es["lethal_save_used"] = true
				accDmg := esInt(es, "accumulated_blocked", 0)
				es["accumulated_blocked"] = 0 // 清空累计

				// 反弹伤害 = 累计格挡量（至少1，确保有意义）
				reflectDmg := accDmg
				if reflectDmg < 1 {
					reflectDmg = 1
				}
				return true, 1, reflectDmg
			},

			UseSkillOverride: func(pts int, es map[string]any) (*SkillResult, int, bool) {
				cfg := HooksConfig("mamori")

				libPtsThreshold := hcInt(cfg, "lib_pts_threshold", 25)
				enhancedPtsThreshold := hcInt(cfg, "enhanced_pts_threshold", 3)
				maxReflectStacks := hcInt(cfg, "max_reflect_stacks", 5)

				if pts >= libPtsThreshold {
					// 解放：全免疫+反弹
					phases := hcInt(cfg, "lib_immune_phases", 10)
					es["lib_immune_phases"] = phases
					cost := hcInt(cfg, "lib_cost", 50)
					return &SkillResult{
						Tier: TierLiberation,
						Desc: fmt.Sprintf("绝对反射：接下来免疫并反弹所有类型伤害（%d个阶段）", phases),
					}, cost, true
				}

				if pts >= enhancedPtsThreshold {
					// 强化：技能免疫
					phases := hcInt(cfg, "enhanced_immune_phases", 3)
					es["skill_immune_phases"] = phases
					cost := hcInt(cfg, "enhanced_cost", 15)
					return &SkillResult{
						Tier: TierEnhanced,
						Desc: fmt.Sprintf("技能障壁：接下来 %d 个阶段免疫技能伤害", phases),
					}, cost, true
				}

				// 普通：增加反弹层数（上限）
				stacks := esInt(es, "reflect_stacks", 0)
				if stacks >= maxReflectStacks {
					return &SkillResult{
						Tier: TierNormal,
						Desc: fmt.Sprintf("镜反护盾已满（%d/%d层），无法继续叠加", stacks, maxReflectStacks),
					}, 0, true
				}
				es["reflect_stacks"] = stacks + 1
				cost := hcInt(cfg, "normal_cost", 10)
				return &SkillResult{
					Tier: TierNormal,
					Desc: fmt.Sprintf("镜反护盾：获得1层反弹（当前%d/%d层），受攻击时消耗1层反弹伤害", stacks+1, maxReflectStacks),
				}, cost, true
			},

			BuildPublicExtra: func(es map[string]any) map[string]any {
				info := map[string]any{}
				if v := esInt(es, "reflect_stacks", 0); v > 0 {
					info["reflect_stacks"] = v
				}
				if esInt(es, "lib_immune_phases", 0) > 0 {
					info["shielded"] = true
				}
				if len(info) == 0 {
					return nil
				}
				return info
			},

			BuildExtraInfo: func(es map[string]any) map[string]any {
				info := map[string]any{}
				if v := esInt(es, "reflect_stacks", 0); v > 0 {
					info["reflect_stacks"] = v
				}
				if v := esInt(es, "accumulated_blocked", 0); v > 0 {
					info["accumulated_blocked"] = v
				}
				if v := esInt(es, "skill_immune_phases", 0); v > 0 {
					info["skill_immune_phases"] = v
				}
				if v := esInt(es, "lib_immune_phases", 0); v > 0 {
					info["lib_immune_phases"] = v
				}
				if esBool(es, "lethal_save_used", false) {
					info["lethal_save_used"] = true
				}
				if len(info) == 0 {
					return nil
				}
				return info
			},
		},
	}
}
