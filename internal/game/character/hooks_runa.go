package character

func init() {
	// 刻印者：出技能牌时根据点数奇偶给对手施加"单/双印记"（栈结构，LIFO），
	// 之后用攻击牌造成奇/偶数伤害可引爆上回合施加的最近同奇偶印记。
	// 单印记引爆：对敌 10 + 自身回血 10
	// 双印记引爆：对敌 20
	// 每回合最多施加 3 层；印记仅保留一回合（下回合结束清除）。
	// 解放被动（累计引爆 3 次后解锁）：每回合开始自动引爆栈顶最近一个印记。
	//
	// ExtraState：
	//   keyin_round         int          —— 角色私有的回合计数（OnPhaseStart action 自增）
	//   marks               []any        —— 印记栈，元素为 map[string]any{parity:"single"/"double", round:int}
	//   marks_this_turn     int          —— 本回合已施加层数
	//   trigger_count       int          —— 累计成功引爆次数
	//   liberation_unlocked bool         —— 解放被动是否已解锁
	//   last_played_type    string       —— 复用 OnCardPlayed/OnDamageDealt 通信
	registry["runa"] = &CharDef{
		ID: "runa",
		Hooks: &CharHooks{
			// 行动阶段开始：推进回合，清理过期印记，重置每回合计数；解放后自动引爆。
			OnPhaseStart: func(phase string, es map[string]any) (int, string) {
				if phase != "action" {
					return 0, ""
				}
				cfg := HooksConfig("runa")
				libCount := hcInt(cfg, "liberation_trigger_count", 3)

				round := esInt(es, "keyin_round", 0) + 1
				es["keyin_round"] = round
				es["marks_this_turn"] = 0

				// 印记仅保留一回合：清除 mark.round <= round-2 的（即"下回合结束清除"）。
				marks := runaGetMarks(es)
				kept := marks[:0]
				for _, m := range marks {
					mr, _ := m["round"].(int)
					if mr >= round-1 {
						kept = append(kept, m)
					}
				}
				es["marks"] = runaAnySlice(kept)

				// 解放被动：引爆栈顶最近一个（必须是上回合，本回合刚施加的不引爆）。
				if esBool(es, "liberation_unlocked", false) || esInt(es, "trigger_count", 0) >= libCount {
					es["liberation_unlocked"] = true
					if idx := runaTopBefore(kept, round); idx >= 0 {
						top := kept[idx]
						kept = append(kept[:idx], kept[idx+1:]...)
						es["marks"] = runaAnySlice(kept)
						runaDetonate(es, top, cfg, "解放·自动引爆")
						es["trigger_count"] = esInt(es, "trigger_count", 0) + 1
						return 0, "解放被动：自动引爆栈顶印记"
					}
				}
				return 0, ""
			},

			// OnCardPlayed：记录原始牌型；技能牌则按奇/偶施加印记（受每回合上限限制）。
			OnCardPlayed: func(cardType string, points int, _ string, es map[string]any) {
				es["last_played_type"] = cardType

				if cardType != "技能" {
					return
				}
				cfg := HooksConfig("runa")
				maxPerTurn := hcInt(cfg, "max_marks_per_turn", 3)
				placed := esInt(es, "marks_this_turn", 0)
				if placed >= maxPerTurn {
					return
				}

				parity := "double"
				if points%2 != 0 {
					parity = "single"
				}
				round := esInt(es, "keyin_round", 0)
				if round == 0 {
					round = 1
					es["keyin_round"] = 1
				}
				marks := runaGetMarks(es)
				marks = append(marks, map[string]any{
					"parity": parity,
					"round":  round,
				})
				es["marks"] = runaAnySlice(marks)
				es["marks_this_turn"] = placed + 1
			},

			// OnDamageDealt：用攻击牌造成奇/偶伤害时，引爆栈顶最近的同奇偶上回合印记。
			OnDamageDealt: func(dmg int, es map[string]any) {
				if dmg <= 0 {
					return
				}
				if esStr(es, "last_played_type", "") != "攻击" {
					return
				}
				cfg := HooksConfig("runa")
				libCount := hcInt(cfg, "liberation_trigger_count", 3)
				round := esInt(es, "keyin_round", 0)
				wantParity := "double"
				if dmg%2 != 0 {
					wantParity = "single"
				}
				marks := runaGetMarks(es)
				idx := -1
				for i := len(marks) - 1; i >= 0; i-- {
					mr, _ := marks[i]["round"].(int)
					if mr >= round {
						continue // 本回合刚施加的不能引爆
					}
					if p, _ := marks[i]["parity"].(string); p == wantParity {
						idx = i
						break
					}
				}
				if idx < 0 {
					return
				}
				m := marks[idx]
				marks = append(marks[:idx], marks[idx+1:]...)
				es["marks"] = runaAnySlice(marks)
				runaDetonate(es, m, cfg, "印记引爆")
				es["trigger_count"] = esInt(es, "trigger_count", 0) + 1
				if esInt(es, "trigger_count", 0) >= libCount {
					es["liberation_unlocked"] = true
				}
			},

			BuildExtraInfo: func(es map[string]any) map[string]any {
				marks := runaGetMarks(es)
				single := 0
				double := 0
				for _, m := range marks {
					if p, _ := m["parity"].(string); p == "single" {
						single++
					} else {
						double++
					}
				}
				return map[string]any{
					"keyin_marks_single":  single,
					"keyin_marks_double":  double,
					"keyin_marks_total":   len(marks),
					"keyin_trigger_count": esInt(es, "trigger_count", 0),
					"keyin_liberation":    esBool(es, "liberation_unlocked", false),
				}
			},

			BuildPublicExtra: func(es map[string]any) map[string]any {
				marks := runaGetMarks(es)
				if len(marks) == 0 && !esBool(es, "liberation_unlocked", false) {
					return nil
				}
				single := 0
				double := 0
				for _, m := range marks {
					if p, _ := m["parity"].(string); p == "single" {
						single++
					} else {
						double++
					}
				}
				return map[string]any{
					"keyin_marks_single": single,
					"keyin_marks_double": double,
					"keyin_liberation":   esBool(es, "liberation_unlocked", false),
				}
			},
		},
	}
}

// runaGetMarks 从 ExtraState 读取印记栈。兼容 []any 与 []map[string]any 两种存储。
func runaGetMarks(es map[string]any) []map[string]any {
	v, ok := es["marks"]
	if !ok || v == nil {
		return nil
	}
	switch arr := v.(type) {
	case []map[string]any:
		return arr
	case []any:
		out := make([]map[string]any, 0, len(arr))
		for _, item := range arr {
			if m, ok := item.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	}
	return nil
}

// runaTopBefore 返回栈中最靠后（最近施加）且 round < currentRound 的印记下标。
// 返回 -1 表示没有可引爆印记（栈空或都是本回合刚施加的）。
func runaTopBefore(marks []map[string]any, currentRound int) int {
	for i := len(marks) - 1; i >= 0; i-- {
		mr, _ := marks[i]["round"].(int)
		if mr < currentRound {
			return i
		}
	}
	return -1
}

// runaDetonate 把一次引爆产生的"对敌伤害 / 自身回血"写进 pending_*。
// engine.applyHookPending 在合适的时机（OnPhaseStart 后 / 攻击落地后）实际执行。
func runaDetonate(es map[string]any, mark map[string]any, cfg map[string]any, label string) {
	parity, _ := mark["parity"].(string)
	if parity == "single" {
		dmg := hcInt(cfg, "single_mark_damage", 10)
		heal := hcInt(cfg, "single_mark_heal", 10)
		if dmg > 0 {
			es["pending_opp_damage"] = esInt(es, "pending_opp_damage", 0) + dmg
			es["pending_opp_damage_label"] = label + "（单印记）"
		}
		if heal > 0 {
			es["pending_heal"] = esInt(es, "pending_heal", 0) + heal
		}
		return
	}
	dmg := hcInt(cfg, "double_mark_damage", 20)
	if dmg > 0 {
		es["pending_opp_damage"] = esInt(es, "pending_opp_damage", 0) + dmg
		es["pending_opp_damage_label"] = label + "（双印记）"
	}
}

// runaAnySlice 将 []map[string]any 装回 []any，便于序列化为客户端 JSON 时统一处理。
func runaAnySlice(marks []map[string]any) []any {
	out := make([]any, 0, len(marks))
	for _, m := range marks {
		out = append(out, m)
	}
	return out
}
