package character

func init() {
	// 积怨者：所有手牌均视为攻击牌；分类累计三种"原始牌型"对外造成的伤害；
	// 任意一类累计达到阈值（默认 50）后，该牌型的攻击转为不可防御的技能伤害，
	// 并在成功落地伤害后补抽两张牌。
	registry["jiyuan"] = &CharDef{
		ID: "jiyuan",
		Hooks: &CharHooks{
			AllCardsAsAttack: true,

			// 出牌时记录"原始牌型"，供后续 IsAttackUndefendable / OnDamageDealt 使用。
			OnCardPlayed: func(cardType string, _ int, es map[string]any) {
				es["last_played_type"] = cardType
			},

			// 在创建 PendingAttack 之前判定：
			// 若本次出牌的"原始牌型"累计伤害已达阈值，本次攻击转为不可防御的技能伤害。
			IsAttackUndefendable: func(es map[string]any) bool {
				cfg := HooksConfig("jiyuan")
				threshold := hcInt(cfg, "unlock_threshold", 50)
				t := esStr(es, "last_played_type", "")
				if t == "" {
					return false
				}
				return esInt(es, "dmg_"+t, 0) >= threshold
			},

			// 累计该牌型造成的最终伤害（仅来自攻击牌路径，技能/反弹/濒死均不计入）。
			OnDamageDealt: func(dmg int, es map[string]any) {
				if dmg <= 0 {
					return
				}
				t := esStr(es, "last_played_type", "")
				if t == "" {
					return
				}
				key := "dmg_" + t
				prev := esInt(es, key, 0)
				es[key] = prev + dmg
			},

			// 已解锁牌型成功造成伤害后补抽 N 张（默认 2）。
			OnAttackHit: func(dmg int, es map[string]any) int {
				cfg := HooksConfig("jiyuan")
				threshold := hcInt(cfg, "unlock_threshold", 50)
				draw := hcInt(cfg, "unlocked_draw", 2)
				t := esStr(es, "last_played_type", "")
				if t == "" || dmg <= 0 {
					return 0
				}
				// 注意：OnDamageDealt 已先一步把 dmg 累加进 dmg_<t>。
				// 因此这里读到的累计值已经"包含"本次伤害。
				if esInt(es, "dmg_"+t, 0) >= threshold {
					return draw
				}
				return 0
			},

			// 自身视图：展示三类累计伤害与解锁状态。
			BuildExtraInfo: func(es map[string]any) map[string]any {
				cfg := HooksConfig("jiyuan")
				threshold := hcInt(cfg, "unlock_threshold", 50)
				return map[string]any{
					"jiyuan_dmg_attack":    esInt(es, "dmg_攻击", 0),
					"jiyuan_dmg_skill":     esInt(es, "dmg_技能", 0),
					"jiyuan_dmg_energy":    esInt(es, "dmg_能耗", 0),
					"jiyuan_unlock_attack": esInt(es, "dmg_攻击", 0) >= threshold,
					"jiyuan_unlock_skill":  esInt(es, "dmg_技能", 0) >= threshold,
					"jiyuan_unlock_energy": esInt(es, "dmg_能耗", 0) >= threshold,
					"jiyuan_threshold":     threshold,
				}
			},

			// 公开视图：仅暴露已解锁牌型给对手，便于战术判断（隐藏具体进度）。
			BuildPublicExtra: func(es map[string]any) map[string]any {
				cfg := HooksConfig("jiyuan")
				threshold := hcInt(cfg, "unlock_threshold", 50)
				return map[string]any{
					"jiyuan_unlock_attack": esInt(es, "dmg_攻击", 0) >= threshold,
					"jiyuan_unlock_skill":  esInt(es, "dmg_技能", 0) >= threshold,
					"jiyuan_unlock_energy": esInt(es, "dmg_能耗", 0) >= threshold,
				}
			},
		},
	}
}

// esStr 从 ExtraState 读取 string，键不存在或类型不符时返回 defVal。
func esStr(es map[string]any, key string, defVal string) string {
	if v, ok := es[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return defVal
}
