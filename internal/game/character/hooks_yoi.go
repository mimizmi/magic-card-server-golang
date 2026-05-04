package character

func init() {
	// 明暗者：所有手牌按攻击牌打出；按"原始牌型"独立累计造成的伤害；
	// 累计能耗牌伤害 ≥ 阈值 → 解锁光形态（每回合行动阶段开始回血）；
	// 累计技能牌伤害 ≥ 阈值 → 解锁暗形态（每回合行动阶段开始对敌方造成伤害）；
	// 同时解锁时两个数值均翻倍。
	registry["yoi"] = &CharDef{
		ID: "yoi",
		Hooks: &CharHooks{
			AllCardsAsAttack: true,

			// 出牌时记录"原始牌型"，与 ruri 共用 ExtraState 键以复用 OnDamageDealt 累计逻辑。
			OnCardPlayed: func(cardType string, _ int, _ string, es map[string]any) {
				es["last_played_type"] = cardType
			},

			// 仅累计"技能"与"能耗"两类原始牌型造成的伤害。
			// 攻击牌产生的伤害无需统计（明暗者只关心两个解锁条件）。
			OnDamageDealt: func(dmg int, es map[string]any) {
				if dmg <= 0 {
					return
				}
				t := esStr(es, "last_played_type", "")
				if t != "技能" && t != "能耗" {
					return
				}
				key := "dmg_" + t
				es[key] = esInt(es, key, 0) + dmg
			},

			// 每个行动阶段开始：根据光/暗解锁状态把"自身回血 / 对敌伤害"写进 pending_*，
			// 由 engine.applyHookPending 实际执行。
			OnPhaseStart: func(phase string, es map[string]any) (int, string) {
				if phase != "action" {
					return 0, ""
				}
				cfg := HooksConfig("yoi")
				threshold := hcInt(cfg, "unlock_threshold", 50)
				lightHeal := hcInt(cfg, "light_heal", 10)
				darkDmg := hcInt(cfg, "dark_damage", 10)
				multiplier := hcInt(cfg, "dual_multiplier", 2)

				lightOn := esInt(es, "dmg_能耗", 0) >= threshold
				darkOn := esInt(es, "dmg_技能", 0) >= threshold
				dual := lightOn && darkOn

				heal := 0
				dmg := 0
				if lightOn {
					heal = lightHeal
				}
				if darkOn {
					dmg = darkDmg
				}
				if dual {
					heal *= multiplier
					dmg *= multiplier
				}

				if heal > 0 {
					es["pending_heal"] = esInt(es, "pending_heal", 0) + heal
				}
				if dmg > 0 {
					es["pending_opp_damage"] = esInt(es, "pending_opp_damage", 0) + dmg
					es["pending_opp_damage_label"] = "暗形态·回合伤害"
				}

				msg := ""
				if dual {
					msg = "明暗双形态：回血 + 对敌伤害（双倍）"
				} else if lightOn {
					msg = "光形态：每回合回血"
				} else if darkOn {
					msg = "暗形态：每回合对敌伤害"
				}
				return 0, msg
			},

			// 自身视图：展示两类累计与光/暗/双形态状态。
			BuildExtraInfo: func(es map[string]any) map[string]any {
				cfg := HooksConfig("yoi")
				threshold := hcInt(cfg, "unlock_threshold", 50)
				lightOn := esInt(es, "dmg_能耗", 0) >= threshold
				darkOn := esInt(es, "dmg_技能", 0) >= threshold
				return map[string]any{
					"mingan_threshold":  threshold,
					"mingan_dmg_skill":  esInt(es, "dmg_技能", 0),
					"mingan_dmg_energy": esInt(es, "dmg_能耗", 0),
					"mingan_light":      lightOn,
					"mingan_dark":       darkOn,
					"mingan_dual":       lightOn && darkOn,
				}
			},

			// 公开视图：仅暴露形态解锁状态，隐藏具体进度。
			BuildPublicExtra: func(es map[string]any) map[string]any {
				cfg := HooksConfig("yoi")
				threshold := hcInt(cfg, "unlock_threshold", 50)
				lightOn := esInt(es, "dmg_能耗", 0) >= threshold
				darkOn := esInt(es, "dmg_技能", 0) >= threshold
				if !lightOn && !darkOn {
					return nil
				}
				return map[string]any{
					"mingan_light": lightOn,
					"mingan_dark":  darkOn,
					"mingan_dual":  lightOn && darkOn,
				}
			},
		},
	}
}
