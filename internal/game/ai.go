package game

import (
	"log/slog"
	"time"

	"echo/internal/game/card"
	"echo/internal/protocol"
)

// ════════════════════════════════════════════════════════════════
//  AI 决策逻辑
//
//  设计原则：
//    - 所有 AI 方法都在 Engine 的 goroutine 中同步执行，无需加锁。
//    - AI 通过直接调用 Engine 内部 handler（handlePlayCard 等），
//      而非通过 actionCh，避免 channel 背压问题。
//    - 小睡眠（time.Sleep）模拟 AI "思考时间"，让对局体验更自然。
// ════════════════════════════════════════════════════════════════

// maybeRunAIAction 检查 AI 是否需要在本次循环迭代中采取行动。
// 返回 true 表示 AI 执行了某个行动（调用方应 continue 进入下一轮检查）。
// 返回 false 表示当前轮到玩家（人类）操作，引擎应进入 select 等待。
func (e *Engine) maybeRunAIAction() bool {
	if e.aiSeat < 0 {
		return false
	}
	aiP := e.state.Players[e.aiSeat]
	if aiP.ActionDone {
		return false
	}

	// 情形 A：防御窗口期，且 AI 是防御方
	if e.state.PendingAttack != nil {
		defSeat := 1 - e.state.PendingAttack.AttackerSeat
		if defSeat == e.aiSeat {
			e.runAIDefense()
			return true
		}
		// 防御窗口期，但 AI 是攻击方 → 等待人类防御
		return false
	}

	// 情形 B：行动阶段，且当前是 AI 的回合
	if e.state.ActiveSeat == e.aiSeat {
		e.runAITurn()
		return true
	}

	return false
}

// runAITurn 执行 AI 的完整行动决策：
//
//  优先级：
//   1. 若可解放（手动解放型且能量足够）→ 触发解放后结束行动
//   2. 若有攻击牌 → 出最高点数攻击牌（触发防御窗口后返回）
//   3. 若有技能牌且能量充足 → 使用技能后结束行动
//   4. 若能量 < 解放阈值且有能耗牌 → 出最高能耗牌后结束行动
//   5. 无合适操作 → 直接结束行动
func (e *Engine) runAITurn() {
	time.Sleep(800 * time.Millisecond) // 模拟思考时间

	seat := e.aiSeat
	p := e.state.Players[seat]

	// 1. 解放
	if p.Char != nil && p.Char.Def.ManualLib && p.Char.CanLiberate(p.Energy) {
		slog.Info("AI triggering liberation", "seat", seat)
		e.handleTriggerLiberate(seat)
		time.Sleep(300 * time.Millisecond)
		e.aiEndAction(seat)
		return
	}

	// 2. 最优攻击牌
	atkZone, atkSlot := e.aiBestAttackCard(seat)
	if atkSlot > 0 {
		slog.Info("AI playing attack card", "seat", seat, "zone", atkZone, "slot", atkSlot)
		payload := protocol.MustEncode(protocol.PlayCardReq{Zone: atkZone, Slot: atkSlot})
		e.handlePlayCard(seat, payload)
		// 若触发了防御窗口，先返回等待人类防御；防御结束后 maybeRunAIAction 会再次调用本函数
		if e.state.PendingAttack != nil {
			return
		}
		// 万能者等将能耗/技能牌视为攻击：打出后无防御窗口，继续结束行动
		time.Sleep(300 * time.Millisecond)
		e.aiEndAction(seat)
		return
	}

	// 3. 技能牌
	skillSlot := e.aiFindSkillCard(seat)
	if skillSlot > 0 && e.aiCanAffordSkill(seat, skillSlot) {
		slog.Info("AI using skill card", "seat", seat, "slot", skillSlot)
		payload := protocol.MustEncode(protocol.UseSkillReq{SkillCardSlot: skillSlot})
		e.handleUseSkill(seat, payload)
		time.Sleep(300 * time.Millisecond)
		e.aiEndAction(seat)
		return
	}

	// 4. 能耗牌（补充能量）
	engZone, engSlot := e.aiBestEnergyCard(seat)
	if engSlot > 0 && p.Energy < p.LibThreshold {
		slog.Info("AI playing energy card", "seat", seat, "zone", engZone, "slot", engSlot)
		payload := protocol.MustEncode(protocol.PlayCardReq{Zone: engZone, Slot: engSlot})
		e.handlePlayCard(seat, payload)
		time.Sleep(300 * time.Millisecond)
		e.aiEndAction(seat)
		return
	}

	// 5. 无合适操作
	slog.Info("AI has no useful action, ending turn", "seat", seat)
	e.aiEndAction(seat)
}

// runAIDefense 处理 AI 的防御决策。
//
//  策略：
//   1. 找点数 ≥ 攻击值的最小牌，完全格挡且最省资源
//   2. 若无法完全格挡，找点数最高的牌尽量减伤
//   3. 若无任何牌，直接放弃防御
func (e *Engine) runAIDefense() {
	time.Sleep(500 * time.Millisecond) // 模拟反应时间

	pending := e.state.PendingAttack
	attackPoints := pending.AttackPoints
	p := e.state.Players[e.aiSeat]

	bestZone, bestSlot, bestPoints := "", 0, 0
	tryCard := func(zone string, slot, pts int) {
		if pts >= attackPoints {
			// 优先找能完全格挡的最小牌
			if bestSlot == 0 || pts < bestPoints || (bestPoints < attackPoints) {
				bestZone, bestSlot, bestPoints = zone, slot, pts
			}
		} else if bestSlot == 0 || (bestPoints < attackPoints && pts > bestPoints) {
			// 退而求其次：找最高牌减少伤害
			bestZone, bestSlot, bestPoints = zone, slot, pts
		}
	}

	for s := 1; s <= card.HandZoneSize; s++ {
		c := p.Hand.HandCard(s)
		if c != nil {
			tryCard("hand", s, c.Points)
		}
	}
	for s := 1; s <= card.SynthZoneSize; s++ {
		c := p.Hand.SynthCard(s)
		if c != nil {
			tryCard("synth", s, c.Points)
		}
	}

	if bestSlot > 0 {
		slog.Info("AI defending", "seat", e.aiSeat,
			"atkPts", attackPoints, "defPts", bestPoints, "zone", bestZone, "slot", bestSlot)
		payload := protocol.MustEncode(protocol.DefenseReq{Zone: bestZone, Slot: bestSlot})
		e.handleDefenseAction(e.aiSeat, payload)
	} else {
		slog.Info("AI passing defense (no cards)", "seat", e.aiSeat)
		payload := protocol.MustEncode(protocol.DefenseReq{Pass: true})
		e.handleDefenseAction(e.aiSeat, payload)
	}
}

// aiEndAction 宣告 AI 结束行动，切换行动权给对手（如果对手未结束）。
func (e *Engine) aiEndAction(seat int) {
	p := e.state.Players[seat]
	p.ActionDone = true
	slog.Info("AI ended action", "seat", seat, "round", e.state.Round)

	opp := e.state.Players[1-seat]
	if !opp.ActionDone {
		e.state.ActiveSeat = 1 - seat
	}
	e.broadcastState("ai ended action")
}

// ════════════════════════════════════════════════════════════════
//  辅助函数：牌型查找
// ════════════════════════════════════════════════════════════════

// aiBestAttackCard 返回 AI 手牌/合成区中点数最高的攻击牌（含万能者被动）。
// 若无攻击牌返回 ("", 0)。
func (e *Engine) aiBestAttackCard(seat int) (zone string, slot int) {
	p := e.state.Players[seat]
	allAsAttack := p.Char != nil && p.Char.Def.Hooks != nil && p.Char.Def.Hooks.AllCardsAsAttack
	best := -1

	for s := 1; s <= card.HandZoneSize; s++ {
		c := p.Hand.HandCard(s)
		if c == nil {
			continue
		}
		if (c.CardType == card.TypeAttack || allAsAttack) && c.Points > best {
			best = c.Points
			zone, slot = "hand", s
		}
	}
	for s := 1; s <= card.SynthZoneSize; s++ {
		c := p.Hand.SynthCard(s)
		if c == nil {
			continue
		}
		if (c.CardType == card.TypeAttack || allAsAttack) && c.Points > best {
			best = c.Points
			zone, slot = "synth", s
		}
	}
	return zone, slot
}

// aiFindSkillCard 返回 AI 手牌区中点数最高的技能牌槽位（1-indexed）。
// 若无技能牌返回 0。
func (e *Engine) aiFindSkillCard(seat int) (slot int) {
	p := e.state.Players[seat]
	best := -1
	for s := 1; s <= card.HandZoneSize; s++ {
		c := p.Hand.HandCard(s)
		if c != nil && c.CardType == card.TypeSkill && c.Points > best {
			best = c.Points
			slot = s
		}
	}
	return slot
}

// aiCanAffordSkill 判断 AI 是否有足够能量使用指定槽位的技能牌。
func (e *Engine) aiCanAffordSkill(seat, skillSlot int) bool {
	p := e.state.Players[seat]
	if p.Char == nil {
		return false
	}
	c := p.Hand.HandCard(skillSlot)
	if c == nil || c.CardType != card.TypeSkill {
		return false
	}
	var cost int
	if c.Points <= 2 {
		cost = p.Char.Def.Normal.EnergyCost
	} else {
		cost = p.Char.Def.Enhanced.EnergyCost
	}
	return p.Energy >= cost
}

// aiBestEnergyCard 返回 AI 手牌区中点数最高的能耗牌。
// 若无能耗牌返回 ("", 0)。
func (e *Engine) aiBestEnergyCard(seat int) (zone string, slot int) {
	p := e.state.Players[seat]
	best := -1
	for s := 1; s <= card.HandZoneSize; s++ {
		c := p.Hand.HandCard(s)
		if c != nil && c.CardType == card.TypeEnergy && c.Points > best {
			best = c.Points
			zone, slot = "hand", s
		}
	}
	return zone, slot
}
