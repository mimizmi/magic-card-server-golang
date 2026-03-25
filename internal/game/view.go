package game

import (
	"echo/internal/game/character"
	"echo/internal/protocol"
)

// BuildView 将服务端完整的 GameState 转化为指定座位玩家能看到的视图。
//
// 这是 Phase 2 设计的信息遮蔽的实装。
// 规则：
//   - 自己的手牌和合成区：完整内容（除非场地效果隐藏点数，Phase 6 补充）
//   - 对手的手牌：只告诉对方有几张（HandCount），不发内容
//   - 对手的合成区：只告诉张数（SynthCount），内容隐藏
//   - 对手角色：未使用技能时显示"???"，使用后公开
func BuildView(gs *GameState, forSeat int) *protocol.GameStateView {
	me := gs.Players[forSeat]
	opp := gs.Players[1-forSeat]

	fieldName := ""
	if gs.FieldEffect != nil {
		fieldName = gs.FieldEffect.Name
	}
	view := &protocol.GameStateView{
		Round:       gs.Round,
		Phase:       string(gs.Phase),
		ActiveSeat:  gs.ActiveSeat,
		FieldEffect: fieldName,
		Me:          buildSelfView(me),
		Opponent:    buildOpponentView(opp),
	}
	if gs.PendingAttack != nil {
		view.PendingAttack = &protocol.PendingAttackView{
			AttackerSeat: gs.PendingAttack.AttackerSeat,
			AttackPoints: gs.PendingAttack.AttackPoints,
		}
	}
	return view
}

// buildSelfView 构建"自己"的完整视图。
func buildSelfView(p *PlayerState) protocol.PlayerView {
	charName := p.CharacterID
	if def, ok := character.Get(p.CharacterID); ok {
		charName = def.Name
	} else if charName == "" {
		charName = "???" // 尚未选择角色
	}

	view := protocol.PlayerView{
		Seat:        p.Seat,
		HP:          p.HP,
		MaxHP:       p.MaxHP,
		Energy:      p.Energy,
		MaxEnergy:   p.MaxEnergy,
		Character:   charName,
		IsNearDeath: p.IsNearDeath,
	}

	// 手牌区：完整内容（含槽位编号，客户端用于 UI 排列）
	for _, sc := range p.Hand.HandSlottedCards() {
		pts := sc.Card.Points
		// Phase 6：若场地效果（虚幻之境·实）使该牌点数隐藏，
		// 对发给自己的视图仍然显示（IsHidden 影响发给对手的视图）
		view.Hand = append(view.Hand, protocol.CardView{
			Slot:     sc.Slot,
			Faction:  sc.Card.SubFaction.String(),
			CardType: sc.Card.CardType.String(),
			Points:   protocol.IntPtr(pts),
		})
	}

	// 合成区：完整内容
	for _, sc := range p.Hand.SynthSlottedCards() {
		pts := sc.Card.Points
		view.SynthZone = append(view.SynthZone, protocol.CardView{
			Slot:     sc.Slot,
			Faction:  sc.Card.SubFaction.String(),
			CardType: sc.Card.CardType.String(),
			Points:   protocol.IntPtr(pts),
		})
	}

	return view
}

// buildOpponentView 构建"对手"的受限视图。
func buildOpponentView(p *PlayerState) protocol.OpponentView {
	// 角色暗置：使用技能前显示 "???"
	charName := "???"
	if p.CharRevealed && p.CharacterID != "" {
		charName = p.CharacterID
	}

	return protocol.OpponentView{
		Seat:        p.Seat,
		HP:          p.HP,
		MaxHP:       p.MaxHP,
		Energy:      p.Energy, // 能量条是公开信息（双方可见）
		MaxEnergy:   p.MaxEnergy,
		Character:   charName,
		IsNearDeath: p.IsNearDeath,
		HandCount:   p.Hand.HandCount(),
		SynthCount:  p.Hand.SynthCount(),
		// 注意：对手的 Hand 和 SynthZone 内容字段根本不存在于 OpponentView 里。
		// 不是空切片，是字段压根不存在，Godot 客户端永远拿不到对手手牌内容。
	}
}
