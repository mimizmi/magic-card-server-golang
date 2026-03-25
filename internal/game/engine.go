package game

import (
	"context"
	"log/slog"
	"math/rand"
	"sync"
	"time"

	"echo/internal/game/card"
	"echo/internal/game/character"
	"echo/internal/game/field"
	"echo/internal/protocol"
	"echo/internal/room"
)

// ════════════════════════════════════════════════════════════════
//  action — 玩家投递给引擎的操作消息
// ════════════════════════════════════════════════════════════════

type action struct {
	Seat    int
	MsgID   uint16
	Payload []byte
}

// ════════════════════════════════════════════════════════════════
//  Engine — 游戏引擎
// ════════════════════════════════════════════════════════════════

// Engine 是一局游戏的权威状态机。
//
// 并发模型：
//   整个游戏逻辑在一个独立的 goroutine（run()）中顺序执行。
//   玩家的网络消息通过 actionCh channel 投递进来，由 run() goroutine 消费。
//
// 为什么选择"单 goroutine 顺序执行"而不是"并发处理每个操作"？
//   游戏状态是高度相关的（一个操作可能影响另一个操作的合法性），
//   串行处理彻底消除了竞态条件，GameState 无需加锁。
//   这是游戏引擎的经典设计，和 Redis 的单线程模型同理。
type Engine struct {
	state    *GameState
	room     *room.Room
	actionCh chan action
	ctx      context.Context
	cancel   context.CancelFunc
	stopOnce sync.Once
	rng      *rand.Rand // 每个引擎独立的随机数源，保证房间间随机独立
}

// NewEngine 创建游戏引擎，绑定到指定房间。
func NewEngine(r *room.Room) *Engine {
	ctx, cancel := context.WithCancel(context.Background())
	return &Engine{
		state:    newGameState(r.ID),
		room:     r,
		actionCh: make(chan action, 32),
		ctx:      ctx,
		cancel:   cancel,
		rng:      rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Start 启动游戏引擎 goroutine，非阻塞。
func (e *Engine) Start() {
	go e.run()
}

// Stop 停止引擎（服务器关闭或强制结束游戏时调用）。
func (e *Engine) Stop() {
	e.stopOnce.Do(e.cancel)
}

// SubmitAction 由网络 Handler 调用，将玩家操作投递给引擎。
// 非阻塞：若引擎已停止，操作被丢弃。
func (e *Engine) SubmitAction(seat int, msgID uint16, payload []byte) {
	select {
	case e.actionCh <- action{Seat: seat, MsgID: msgID, Payload: payload}:
	case <-e.ctx.Done():
	}
}

// ════════════════════════════════════════════════════════════════
//  主循环
// ════════════════════════════════════════════════════════════════

func (e *Engine) run() {
	defer e.Stop()
	slog.Info("engine started", "gameID", e.state.GameID)

	// 步骤 1：等待双方选择角色
	if !e.waitCharacterSelect() {
		return
	}

	// 步骤 2：初始发牌（8张），通知游戏开始
	e.doDraw()
	e.broadcastState("game start")

	// 主阶段循环
	for {
		e.state.Round++
		slog.Info("round start", "gameID", e.state.GameID, "round", e.state.Round)

		// ── 第一步：场地效果（第2回合起）──────────────────────
		if e.state.Round >= 2 {
			e.runFieldDraw()
		}

		// ── 第二步：补牌 ───────────────────────────────────────
		e.runDraw()

		// ── 第三步：行动 ───────────────────────────────────────
		if !e.runAction() {
			return // ctx 被取消（服务器关闭）
		}

		// ── 第四步：交战结算 ───────────────────────────────────
		e.runCombat()

		// ── 第五步：清场 ───────────────────────────────────────
		if e.runCleanup() {
			return // 游戏结束
		}
	}
}

// ════════════════════════════════════════════════════════════════
//  各阶段实现
// ════════════════════════════════════════════════════════════════

// waitCharacterSelect 阻塞直到双方均发送 SelectCharacterReq。
func (e *Engine) waitCharacterSelect() bool {
	e.state.Phase = PhaseWaiting
	selected := [2]bool{}

	for !(selected[0] && selected[1]) {
		select {
		case act := <-e.actionCh:
			if act.MsgID != protocol.MsgSelectCharacterReq {
				continue
			}
			req, err := protocol.Decode[protocol.SelectCharacterReq](act.Payload)
			if err != nil || req.CharacterID == "" {
				e.sendError(act.Seat, protocol.ErrCodeInvalidPhase, "无效的角色选择")
				continue
			}
			inst, err := character.NewInstance(req.CharacterID)
			if err != nil {
				e.sendError(act.Seat, protocol.ErrCodeInvalidPhase, "未知角色："+req.CharacterID)
				continue
			}
			p := e.state.Players[act.Seat]
			p.Char = inst
			p.CharacterID = inst.Def.ID
			// 用角色定义覆盖玩家初始属性
			p.MaxHP = inst.Def.MaxHP
			p.HP = inst.Def.MaxHP
			p.MaxEnergy = inst.Def.MaxEnergy
			p.LibThreshold = inst.Def.LibThreshold
			selected[act.Seat] = true
			slog.Info("character selected", "seat", act.Seat, "char", req.CharacterID)

		case <-e.ctx.Done():
			return false
		}
	}

	// 通知游戏正式开始（角色暗置，双方显示 "???"）
	e.room.Broadcast(protocol.MsgGameStartEv, protocol.MustEncode(protocol.GameStartEv{
		GameID:    e.state.GameID,
		Seat0Char: "???",
		Seat1Char: "???",
	}))
	return true
}

// runFieldDraw 第2回合起抽取场地效果，通知双方。
func (e *Engine) runFieldDraw() {
	e.state.Phase = PhaseFieldDraw
	e.state.FieldEffect = field.Draw(e.rng)
	slog.Info("field effect drawn",
		"gameID", e.state.GameID,
		"round", e.state.Round,
		"effect", e.state.FieldEffect.Name,
	)
	e.broadcastPhaseChange()
}

// runDraw 补牌阶段：每位玩家补至8张（濒死补至4张）。
func (e *Engine) runDraw() {
	e.state.Phase = PhaseDraw
	e.broadcastPhaseChange()
	e.doDraw()

	// 虚幻之境·虚：本回合补入的牌对对手隐藏
	if e.state.FieldEffect != nil && e.state.FieldEffect.HideDrawnCards {
		for _, p := range e.state.Players {
			for _, sc := range p.Hand.HandSlottedCards() {
				sc.Card.IsHidden = true
			}
		}
	}

	e.broadcastState("draw phase")
}

// doDraw 执行实际补牌操作（初始发牌和补牌阶段共用）。
func (e *Engine) doDraw() {
	for _, p := range e.state.Players {
		p.drawCards()
	}
}

// runAction 行动阶段：轮换制，Seat 0 先行动，结束后换 Seat 1，双方均结束后进入下一阶段。
// 返回 false 表示 ctx 被取消（服务器关闭），应退出 run()。
func (e *Engine) runAction() bool {
	e.state.Phase = PhaseAction
	e.state.ActiveSeat = 0 // Seat 0 先手
	e.state.Players[0].ActionDone = false
	e.state.Players[1].ActionDone = false

	e.broadcastPhaseChange()
	e.broadcastState("action phase start")

	for {
		// 双方都结束行动
		if e.state.Players[0].ActionDone && e.state.Players[1].ActionDone {
			return true
		}

		select {
		case act := <-e.actionCh:
			e.processAction(act)
		case <-e.ctx.Done():
			return false
		}
	}
}

// processAction 处理一条玩家行动消息，所有操作在此串行执行。
func (e *Engine) processAction(act action) {
	// 防御窗口期间：即使防御方已宣告结束行动，仍必须响应来袭攻击
	if e.state.PendingAttack != nil {
		defSeat := 1 - e.state.PendingAttack.AttackerSeat
		if act.Seat != defSeat {
			e.sendError(act.Seat, protocol.ErrCodeInvalidPhase, "等待对手防御中")
			return
		}
		if act.MsgID != protocol.MsgDefenseReq {
			e.sendError(act.Seat, protocol.ErrCodeInvalidPhase, "请先响应来袭攻击（出牌防御或放弃防御）")
			return
		}
		e.handleDefenseAction(act.Seat, act.Payload)
		return
	}

	p := e.state.Players[act.Seat]

	// 已宣告结束的玩家不能再操作
	if p.ActionDone {
		e.sendError(act.Seat, protocol.ErrCodeInvalidPhase, "你已宣告行动结束")
		return
	}

	// 轮换制：非当前行动方不能操作
	if act.Seat != e.state.ActiveSeat {
		e.sendError(act.Seat, protocol.ErrCodeInvalidPhase, "还没到你的回合")
		return
	}

	switch act.MsgID {
	case protocol.MsgPlayCardReq:
		e.handlePlayCard(act.Seat, act.Payload)
	case protocol.MsgMoveToSynthReq:
		e.handleMoveToSynth(act.Seat, act.Payload)
	case protocol.MsgSynthesizeReq:
		e.handleSynthesize(act.Seat, act.Payload)
	case protocol.MsgEndActionReq:
		p.ActionDone = true
		slog.Info("player ended action", "seat", act.Seat, "round", e.state.Round)
		// 轮换制：切换行动权给对手（如果对手未结束）
		opp := e.state.Players[1-act.Seat]
		if !opp.ActionDone {
			e.state.ActiveSeat = 1 - act.Seat
			slog.Info("active seat switched", "newActiveSeat", e.state.ActiveSeat)
		}
		e.broadcastState("player ended action")
	case protocol.MsgUseSkillReq:
		e.handleUseSkill(act.Seat, act.Payload)
	case protocol.MsgTriggerLibrateReq:
		e.handleTriggerLiberate(act.Seat)
	default:
		slog.Warn("unknown action msgID in action phase", "msgID", act.MsgID)
	}
}

// handlePlayCard 处理打出一张牌的请求。
func (e *Engine) handlePlayCard(seat int, payload []byte) {
	req, err := protocol.Decode[protocol.PlayCardReq](payload)
	if err != nil {
		e.sendError(seat, protocol.ErrCodeInvalidSlot, "无效请求格式")
		return
	}

	p := e.state.Players[seat]
	var c *card.Card

	// 从指定区域取牌
	switch req.Zone {
	case "hand":
		c, err = p.Hand.TakeHand(req.Slot)
	case "synth":
		c, err = p.Hand.TakeSynth(req.Slot)
	default:
		e.sendError(seat, protocol.ErrCodeInvalidSlot, "无效区域："+req.Zone)
		return
	}

	if err != nil {
		e.sendError(seat, protocol.ErrCodeInvalidSlot, err.Error())
		return
	}
	if c == nil {
		e.sendError(seat, protocol.ErrCodeNoCard, "该槽位没有牌")
		return
	}

	switch c.CardType {
	case card.TypeAttack:
		// 攻击牌：即时结算（三国杀式），打出后对手进入防御窗口
		attackPoints := c.Points
		attackPoints = e.applyOutgoing(p, attackPoints)
		if e.state.FieldEffect != nil {
			attackPoints += e.state.FieldEffect.BonusAttack
		}
		defSeat := 1 - seat
		e.state.PendingAttack = &PendingAttack{AttackerSeat: seat, AttackPoints: attackPoints}
		e.state.ActiveSeat = defSeat
		slog.Info("attack played, defense window opened",
			"seat", seat, "attackPoints", attackPoints, "defSeat", defSeat)
		e.room.Broadcast(protocol.MsgIncomingAttackEv, protocol.MustEncode(protocol.IncomingAttackEv{
			AttackerSeat: seat, AttackPoints: attackPoints,
		}))
		e.broadcastState("attack pending, awaiting defense")

	case card.TypeEnergy:
		// 能耗牌：转化为能量
		gained := c.Points
		p.Energy = min(p.Energy+gained, p.MaxEnergy)
		slog.Info("energy gained", "seat", seat, "gained", gained, "total", p.Energy)
		e.sendPlayerStatus(seat)
		e.sendStateTo(seat, "energy card played") // 更新手牌区（移除已打出的能耗牌）

		// 能量达到解放阈值时通知（Phase 7 处理实际触发逻辑）
		if p.Energy >= p.LibThreshold {
			slog.Info("liberation threshold reached", "seat", seat, "energy", p.Energy)
		}

	case card.TypeSkill:
		// 技能牌不通过 PlayCardReq 触发，放回原槽并提示正确用法
		_ = p.Hand.PlaceHand(req.Slot, c)
		e.sendError(seat, protocol.ErrCodeInvalidPhase, "请通过「使用技能」(UseSkillReq) 触发技能牌")
	}
}

// handleMoveToSynth 将手牌移入合成区。
func (e *Engine) handleMoveToSynth(seat int, payload []byte) {
	req, err := protocol.Decode[protocol.MoveToSynthReq](payload)
	if err != nil {
		e.sendError(seat, protocol.ErrCodeInvalidSlot, "无效请求格式")
		return
	}
	if err := e.state.Players[seat].Hand.MoveToSynth(req.HandSlot); err != nil {
		e.sendError(seat, protocol.ErrCodeInvalidSlot, err.Error())
		return
	}
	e.sendStateTo(seat, "move to synth")
	e.sendStateTo(1-seat, "opponent move to synth") // 更新对手合成区张数
}

// handleSynthesize 执行合成操作。
func (e *Engine) handleSynthesize(seat int, payload []byte) {
	req, err := protocol.Decode[protocol.SynthesizeReq](payload)
	if err != nil {
		e.sendError(seat, protocol.ErrCodeInvalidSlot, "无效请求格式")
		return
	}

	opts := e.fieldSynthOpts()

	result, err := e.state.Players[seat].Hand.SynthesizeCards(
		req.Zone1, req.Slot1,
		req.Zone2, req.Slot2,
		opts,
	)
	if err != nil {
		code := protocol.ErrCodeInvalidSlot
		if err == card.ErrSameCardType {
			code = protocol.ErrCodeSynthSameType
		}
		e.sendError(seat, code, err.Error())
		return
	}

	slog.Info("synthesis done", "seat", seat, "result", result.String())
	e.sendStateTo(seat, "synthesize")
	e.sendStateTo(1-seat, "opponent synthesize") // 更新对手合成区张数
}

// runCombat 交战结算阶段。
// 三国杀式即时结算：攻击已在行动阶段逐一结算，此阶段仅作阶段标记。
func (e *Engine) runCombat() {
	e.state.Phase = PhaseCombat
	e.broadcastPhaseChange()
	e.broadcastState("combat phase")
}

// handleDefenseAction 处理防御窗口期内的 DefenseReq。
// 防御方可以打出一张牌抵消对应点数的伤害，或 Pass 承受全部伤害。
func (e *Engine) handleDefenseAction(seat int, payload []byte) {
	req, err := protocol.Decode[protocol.DefenseReq](payload)
	if err != nil {
		e.sendError(seat, protocol.ErrCodeInvalidSlot, "无效请求格式")
		return
	}

	pending := e.state.PendingAttack
	attackPoints := pending.AttackPoints
	atkSeat := pending.AttackerSeat

	// 清除防御窗口，归还行动权给攻击方
	e.state.PendingAttack = nil
	e.state.ActiveSeat = atkSeat

	if req.Pass {
		// 放弃防御：承受全部伤害
		slog.Info("defense passed", "seat", seat, "damage", attackPoints)
		e.applyDamage(seat, attackPoints, "攻击牌伤害")
		e.broadcastState("defense passed")
		return
	}

	// 出牌防御：取出指定牌
	p := e.state.Players[seat]
	var defCard *card.Card
	switch req.Zone {
	case "hand":
		defCard, err = p.Hand.TakeHand(req.Slot)
	case "synth":
		defCard, err = p.Hand.TakeSynth(req.Slot)
	default:
		// 区域非法，回退到 Pass 处理
		e.applyDamage(seat, attackPoints, "攻击牌伤害")
		e.broadcastState("defense invalid zone, treated as pass")
		return
	}
	if err != nil || defCard == nil {
		// 槽位无牌，同 Pass 处理
		e.applyDamage(seat, attackPoints, "攻击牌伤害")
		e.broadcastState("defense card missing, treated as pass")
		return
	}

	remaining := attackPoints - defCard.Points
	slog.Info("defense card played",
		"seat", seat, "defPoints", defCard.Points,
		"atkPoints", attackPoints, "remaining", remaining)

	if remaining > 0 {
		e.applyDamage(seat, remaining, "攻击牌伤害（防御后剩余）")
	}
	e.broadcastState("defense resolved")
}

// applyDamage 对目标玩家造成伤害，处理濒死逻辑，推送伤害事件。
func (e *Engine) applyDamage(targetSeat, amount int, detail string) {
	p := e.state.Players[targetSeat]

	// 被动：受击方角色减免（主角色 + 赐福第二角色叠加）
	finalDamage := amount
	if p.Char != nil {
		finalDamage = p.Char.ModifyIncoming(finalDamage)
	}
	if p.SecondChar != nil {
		finalDamage = p.SecondChar.ModifyIncoming(finalDamage)
	}

	p.HP -= finalDamage

	hpAfter := max(p.HP, 0)
	e.room.Broadcast(protocol.MsgDamageEv, protocol.MustEncode(protocol.DamageEv{
		AttackerSeat: 1 - targetSeat,
		DefenderSeat: targetSeat,
		RawDamage:    amount,
		FinalDamage:  finalDamage,
		HPAfter:      hpAfter,
		Detail:       detail,
	}))

	if p.HP <= 0 {
		e.handleHPZero(targetSeat)
	}
}

// handleHPZero 处理 HP 归零，实现濒死机制。
func (e *Engine) handleHPZero(seat int) {
	p := e.state.Players[seat]

	if !p.IsNearDeath {
		// 首次归零：进入濒死状态，回血到60
		p.HP = 60
		p.IsNearDeath = true
		slog.Info("player entered near-death", "seat", seat)
		e.sendPlayerStatus(seat)
		e.room.Broadcast(protocol.MsgPlayerStatusEv, protocol.MustEncode(protocol.PlayerStatusEv{
			Seat: seat, HP: 60, MaxHP: p.MaxHP,
			Energy: p.Energy, MaxEnergy: p.MaxEnergy,
		}))
	} else {
		// 二次归零：检查殉道者被动拦截
		if p.Char != nil && p.Char.InterceptSecondDeath() {
			// 殉道者解放自动触发：HP 回至60，广播解放事件
			p.HP = 60
			slog.Info("martyr liberation triggered", "seat", seat)
			e.room.Broadcast(protocol.MsgLiberationEv, protocol.MustEncode(protocol.LiberationEv{
				PlayerSeat: seat,
				Character:  p.CharacterID,
				Desc:       p.Char.Def.Lib.Result.Desc,
			}))
			result := p.Char.Def.Lib.Result
			e.applySkillResult(seat, &result)
			p.CharRevealed = true
		} else {
			// 真正死亡
			p.HP = 0
			e.triggerDeath(seat)
		}
	}
}

// triggerDeath 判定玩家死亡，确定胜负。
func (e *Engine) triggerDeath(loseSeat int) {
	winner := 1 - loseSeat
	e.state.Phase = PhaseGameOver
	e.state.Winner = winner

	slog.Info("game over", "gameID", e.state.GameID, "winner", winner)
	e.room.Broadcast(protocol.MsgGameOverEv, protocol.MustEncode(protocol.GameOverEv{
		WinnerSeat: winner,
		Reason:     "hp_zero",
	}))
}

// runCleanup 清场阶段，返回 true 表示游戏已结束。
func (e *Engine) runCleanup() bool {
	e.state.Phase = PhaseCleanup
	e.broadcastPhaseChange()

	nearDeathDrain := 30
	if e.state.FieldEffect != nil {
		nearDeathDrain = e.state.FieldEffect.ActualNearDeathDrain()
	}

	for seat, p := range e.state.Players {
		// 濒死扣血（守护之光下 15，默认 30）
		if p.IsNearDeath {
			e.applyDamage(seat, nearDeathDrain, "濒死扣除")
			if e.state.isOver() {
				return true
			}
		}

		// 清除弃牌区（手牌区槽位 5-8）
		p.Hand.ClearDiscardZone()

		// 赐福判定：HP < 40 且尚未触发
		if p.HP < 40 && !p.BlessingUsed {
			p.BlessingUsed = true
			second := e.assignBlessingChar(seat)
			p.SecondChar = second
			slog.Info("blessing triggered",
				"seat", seat, "hp", p.HP,
				"secondChar", second.Def.Name,
			)
			e.room.Broadcast(protocol.MsgBlessingEv, protocol.MustEncode(protocol.BlessingEv{
				PlayerSeat:     seat,
				SecondCharID:   second.Def.ID,
				SecondCharName: second.Def.Name,
			}))
		}
	}

	e.broadcastState("cleanup done")
	return e.state.isOver()
}

// ════════════════════════════════════════════════════════════════
//  广播辅助函数
// ════════════════════════════════════════════════════════════════

// broadcastState 向双方各发送其专属的游戏状态视图（信息遮蔽后）。
// reason 仅用于日志，不发给客户端。
func (e *Engine) broadcastState(reason string) {
	slog.Debug("broadcast state", "gameID", e.state.GameID, "reason", reason)
	for seat := range e.state.Players {
		view := BuildView(e.state, seat)
		e.room.SendTo(seat, protocol.MsgGameStateEv, protocol.MustEncode(view))
	}
}

// sendStateTo 只给指定座位的玩家发送其视图（操作反馈，不需要通知对手）。
func (e *Engine) sendStateTo(seat int, reason string) {
	view := BuildView(e.state, seat)
	e.room.SendTo(seat, protocol.MsgGameStateEv, protocol.MustEncode(view))
}

// broadcastPhaseChange 广播阶段切换事件（含当前场地效果名称和行动方）。
func (e *Engine) broadcastPhaseChange() {
	fieldName := ""
	if e.state.FieldEffect != nil {
		fieldName = e.state.FieldEffect.Name
	}
	ev := protocol.MustEncode(protocol.PhaseChangeEv{
		Round:       e.state.Round,
		Phase:       string(e.state.Phase),
		ActiveSeat:  e.state.ActiveSeat,
		FieldEffect: fieldName,
	})
	e.room.Broadcast(protocol.MsgPhaseChangeEv, ev)
}

// applyOutgoing 对攻击伤害依次应用主角色和赐福角色的 BonusOutgoing 被动。
func (e *Engine) applyOutgoing(p *PlayerState, dmg int) int {
	if p.Char != nil {
		dmg = p.Char.ModifyOutgoing(dmg)
	}
	if p.SecondChar != nil {
		dmg = p.SecondChar.ModifyOutgoing(dmg)
	}
	return dmg
}

// assignBlessingChar 从角色池中随机选取一个与当前角色不同的角色作为赐福角色。
func (e *Engine) assignBlessingChar(seat int) *character.CharInstance {
	primaryID := e.state.Players[seat].CharacterID
	all := character.All()

	// 过滤掉主角色，再随机选一个
	candidates := make([]*character.CharDef, 0, len(all)-1)
	for _, def := range all {
		if def.ID != primaryID {
			candidates = append(candidates, def)
		}
	}
	if len(candidates) == 0 {
		// 极端情况：只有一个角色，随机返回任意（不应发生）
		candidates = all
	}
	chosen := candidates[e.rng.Intn(len(candidates))]
	inst, _ := character.NewInstance(chosen.ID)
	return inst
}

// fieldSynthOpts 将当前场地效果转换为合成配置。
// 无场地效果时返回 DefaultOpts。
func (e *Engine) fieldSynthOpts() card.SynthesisOpts {
	opts := card.DefaultOpts()
	fe := e.state.FieldEffect
	if fe == nil {
		return opts
	}

	opts.IllusionBonus = fe.IllusionBonus
	opts.AllowSameType = fe.AllowSameType

	// 将 field.ReincarnHint 转换为 card.ReincarnationRule
	switch fe.ReincarnRule {
	case field.ReincAsBase:
		opts.ReincarnationRule = card.ReincarnationAsBase
	case field.ReincAsOther:
		opts.ReincarnationRule = card.ReincarnationAsOther
	default:
		opts.ReincarnationRule = card.ReincarnationNormal
	}

	return opts
}

// sendPlayerStatus 向双方推送某玩家 HP/能量的增量更新。
func (e *Engine) sendPlayerStatus(seat int) {
	p := e.state.Players[seat]
	ev := protocol.MustEncode(protocol.PlayerStatusEv{
		Seat:      seat,
		HP:        p.HP,
		MaxHP:     p.MaxHP,
		Energy:    p.Energy,
		MaxEnergy: p.MaxEnergy,
	})
	e.room.Broadcast(protocol.MsgPlayerStatusEv, ev)
}

// sendError 向指定玩家发送错误消息（操作被拒绝时）。
func (e *Engine) sendError(seat, code int, msg string) {
	e.room.SendTo(seat, protocol.MsgErrorEv, protocol.MustEncode(protocol.ErrorEv{
		Code:    code,
		Message: msg,
	}))
}

// ════════════════════════════════════════════════════════════════
//  角色技能处理
// ════════════════════════════════════════════════════════════════

// handleUseSkill 处理 MsgUseSkillReq：消耗技能牌，激活普通/强化技能。
func (e *Engine) handleUseSkill(seat int, payload []byte) {
	req, err := protocol.Decode[protocol.UseSkillReq](payload)
	if err != nil {
		e.sendError(seat, protocol.ErrCodeInvalidSlot, "无效请求格式")
		return
	}

	p := e.state.Players[seat]
	if p.Char == nil {
		e.sendError(seat, protocol.ErrCodeInvalidPhase, "尚未选择角色")
		return
	}

	// 从手牌取出技能牌
	c, err := p.Hand.TakeHand(req.SkillCardSlot)
	if err != nil {
		e.sendError(seat, protocol.ErrCodeInvalidSlot, err.Error())
		return
	}
	if c == nil {
		e.sendError(seat, protocol.ErrCodeNoCard, "该槽位没有牌")
		return
	}
	if c.CardType != card.TypeSkill {
		// 不是技能牌，放回原槽
		_ = p.Hand.PlaceHand(req.SkillCardSlot, c)
		e.sendError(seat, protocol.ErrCodeInvalidSlot, "该槽位不是技能牌")
		return
	}

	// 根据牌点数决定技能档位，检查能量
	result, cost, err := p.Char.UseSkill(c.Points)
	if err != nil {
		_ = p.Hand.PlaceHand(req.SkillCardSlot, c)
		e.sendError(seat, protocol.ErrCodeInvalidPhase, err.Error())
		return
	}
	if p.Energy < cost {
		_ = p.Hand.PlaceHand(req.SkillCardSlot, c)
		e.sendError(seat, protocol.ErrCodeInvalidPhase, "能量不足")
		return
	}

	// 扣除能量，技能牌消耗（已从手牌取出，直接丢弃）
	p.Energy -= cost
	p.CharRevealed = true

	// 广播技能使用事件（公开角色身份）
	e.room.Broadcast(protocol.MsgSkillUsedEv, protocol.MustEncode(protocol.SkillUsedEv{
		PlayerSeat: seat,
		Character:  p.CharacterID,
		SkillLevel: int(result.Tier),
		Desc:       result.Desc,
	}))

	e.applySkillResult(seat, result)
	e.broadcastState("skill used")
}

// handleTriggerLiberate 处理 MsgTriggerLibrateReq：手动触发解放技能。
// 殉道者不使用此消息（自动触发），其他5个角色均可手动触发。
func (e *Engine) handleTriggerLiberate(seat int) {
	p := e.state.Players[seat]
	if p.Char == nil {
		e.sendError(seat, protocol.ErrCodeInvalidPhase, "尚未选择角色")
		return
	}
	if !p.Char.Def.ManualLib {
		e.sendError(seat, protocol.ErrCodeInvalidPhase, "该角色的解放为自动触发")
		return
	}
	if !p.Char.CanLiberate(p.Energy) {
		if p.Char.LibUsed {
			e.sendError(seat, protocol.ErrCodeInvalidPhase, "解放技能已使用过")
		} else {
			e.sendError(seat, protocol.ErrCodeInvalidPhase, "能量不足以触发解放")
		}
		return
	}

	// 扣除解放所需能量
	p.Energy -= p.Char.Def.LibThreshold

	result, _ := p.Char.TriggerLiberation() // LibUsed 在此内部标记
	p.CharRevealed = true

	e.room.Broadcast(protocol.MsgLiberationEv, protocol.MustEncode(protocol.LiberationEv{
		PlayerSeat: seat,
		Character:  p.CharacterID,
		Desc:       result.Desc,
	}))

	e.applySkillResult(seat, result)
	e.broadcastState("liberation triggered")
}

// applySkillResult 将技能产生的效果应用到 GameState。
// 统一处理所有技能效果，避免在各处重复逻辑。
func (e *Engine) applySkillResult(seat int, result *character.SkillResult) {
	p := e.state.Players[seat]
	opp := e.state.Players[1-seat]

	if result.HealSelf > 0 {
		p.HP = min(p.HP+result.HealSelf, p.MaxHP)
	}
	if result.GainEnergy > 0 {
		p.Energy = min(p.Energy+result.GainEnergy, p.MaxEnergy)
	}
	if result.DrawCards > 0 {
		p.Hand.DrawIntoHand(p.Deck, result.DrawCards)
	}
	if result.DealDirectDamage > 0 {
		// 直接伤害：不走攻击牌路径，仍经过受击方被动减免
		oppSeat := 1 - seat
		dmg := result.DealDirectDamage
		if opp.Char != nil {
			dmg = opp.Char.ModifyIncoming(dmg)
		}
		opp.HP -= dmg
		e.room.Broadcast(protocol.MsgDamageEv, protocol.MustEncode(protocol.DamageEv{
			AttackerSeat: seat,
			DefenderSeat: oppSeat,
			RawDamage:    result.DealDirectDamage,
			FinalDamage:  dmg,
			HPAfter:      max(opp.HP, 0),
			Detail:       "技能直接伤害",
		}))
		if opp.HP <= 0 {
			e.handleHPZero(oppSeat)
		}
	}
	// 同步状态（能量/HP）
	e.sendPlayerStatus(seat)
}
