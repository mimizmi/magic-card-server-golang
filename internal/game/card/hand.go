package card

import (
	"errors"
	"fmt"
)

// ════════════════════════════════════════════════════════════════
//  手牌区与合成区
// ════════════════════════════════════════════════════════════════

const (
	HandZoneSize  = 8 // 手牌区总槽位数
	SafeZoneSize  = 4 // 安全区（槽位 1-4），阶段结束后保留
	SynthZoneSize = 4 // 合成区总槽位数
)

// HandZone 管理一名玩家的手牌区（8槽）和合成区（4槽）。
//
// 槽位约定（均为 1-indexed，对外 API 用 1-8 / 1-4）：
//   手牌区 槽位 1-4 = 安全区（safeZone），阶段结束后不清除
//   手牌区 槽位 5-8 = 弃牌区（discardZone），阶段结束时强制清除
//   合成区 槽位 1-4 = synthZone，不随阶段结束自动清除
//
// 内部用 0-indexed 数组存储，对外接口全部用 1-indexed。
// nil 表示该槽位为空。
type HandZone struct {
	hand  [HandZoneSize]*Card  // hand[0..3] = 安全区，hand[4..7] = 弃牌区
	synth [SynthZoneSize]*Card // 合成区
}

// NewHandZone 创建空手牌区。
func NewHandZone() *HandZone {
	return &HandZone{}
}

// ════════════════════════════════════════════════════════════════
//  手牌区操作
// ════════════════════════════════════════════════════════════════

// HandCard 返回手牌区指定槽位的牌（1-indexed），空槽返回 nil。
func (h *HandZone) HandCard(slot int) *Card {
	if !validHandSlot(slot) {
		return nil
	}
	return h.hand[slot-1]
}

// PlaceHand 将牌放入手牌区指定槽位。槽位已有牌时报错。
func (h *HandZone) PlaceHand(slot int, c *Card) error {
	if !validHandSlot(slot) {
		return fmt.Errorf("无效手牌槽位: %d", slot)
	}
	if h.hand[slot-1] != nil {
		return fmt.Errorf("手牌槽位 %d 已有牌", slot)
	}
	h.hand[slot-1] = c
	return nil
}

// TakeHand 从手牌区取出指定槽位的牌（取出后槽位变空）。
// 槽位为空时返回 nil, nil（不报错，调用方按需判断）。
func (h *HandZone) TakeHand(slot int) (*Card, error) {
	if !validHandSlot(slot) {
		return nil, fmt.Errorf("无效手牌槽位: %d", slot)
	}
	c := h.hand[slot-1]
	h.hand[slot-1] = nil
	return c, nil
}

// Fill 将牌堆中的牌补充到手牌区，直到填满 maxSlots 个槽位（或手牌区填满）。
// maxSlots = 8 为正常状态；濒死状态下为 4（只填安全区）。
func (h *HandZone) Fill(deck *Deck, maxSlots int) {
	if maxSlots > HandZoneSize {
		maxSlots = HandZoneSize
	}
	for i := 0; i < maxSlots; i++ {
		if h.hand[i] == nil {
			h.hand[i] = deck.Draw()
		}
	}
}

// HandCount 返回手牌区当前的牌张数（不含空槽）。
func (h *HandZone) HandCount() int {
	count := 0
	for _, c := range h.hand {
		if c != nil {
			count++
		}
	}
	return count
}

// AllHandCards 返回手牌区所有非空牌的切片（保留槽位顺序）。
// 用于向客户端发送手牌视图。
func (h *HandZone) AllHandCards() []*Card {
	cards := make([]*Card, 0, HandZoneSize)
	for _, c := range h.hand {
		if c != nil {
			cards = append(cards, c)
		}
	}
	return cards
}

// HandSlotOf 返回指定牌 ID 在手牌区的槽位（1-indexed），找不到返回 0。
func (h *HandZone) HandSlotOf(cardID string) int {
	for i, c := range h.hand {
		if c != nil && c.ID == cardID {
			return i + 1
		}
	}
	return 0
}

// ════════════════════════════════════════════════════════════════
//  合成区操作
// ════════════════════════════════════════════════════════════════

// SynthCard 返回合成区指定槽位的牌（1-indexed）。
func (h *HandZone) SynthCard(slot int) *Card {
	if !validSynthSlot(slot) {
		return nil
	}
	return h.synth[slot-1]
}

// TakeSynth 从合成区取出指定槽位的牌。
func (h *HandZone) TakeSynth(slot int) (*Card, error) {
	if !validSynthSlot(slot) {
		return nil, fmt.Errorf("无效合成区槽位: %d", slot)
	}
	c := h.synth[slot-1]
	h.synth[slot-1] = nil
	return c, nil
}

// PutSynth 将牌放入合成区第一个空槽。合成区满时报错。
func (h *HandZone) PutSynth(c *Card) error {
	for i, s := range h.synth {
		if s == nil {
			h.synth[i] = c
			return nil
		}
	}
	return errors.New("合成区已满（最多 4 张）")
}

// SynthCount 返回合成区当前的牌张数。
func (h *HandZone) SynthCount() int {
	count := 0
	for _, c := range h.synth {
		if c != nil {
			count++
		}
	}
	return count
}

// AllSynthCards 返回合成区所有非空牌（保留槽位顺序）。
func (h *HandZone) AllSynthCards() []*Card {
	cards := make([]*Card, 0, SynthZoneSize)
	for _, c := range h.synth {
		if c != nil {
			cards = append(cards, c)
		}
	}
	return cards
}

// ════════════════════════════════════════════════════════════════
//  跨区操作
// ════════════════════════════════════════════════════════════════

// MoveToSynth 将手牌区 handSlot 的牌移入合成区。
// 安全区（1-4）和弃牌区（5-8）的牌均可移入。
// 合成区满、或指定手牌槽位为空时报错。
func (h *HandZone) MoveToSynth(handSlot int) error {
	c, err := h.TakeHand(handSlot)
	if err != nil {
		return err
	}
	if c == nil {
		return fmt.Errorf("手牌槽位 %d 为空", handSlot)
	}
	if err := h.PutSynth(c); err != nil {
		// 合成区满，原路放回
		h.hand[handSlot-1] = c
		return err
	}
	return nil
}

// SynthesizeCards 执行一次合成操作：
//   - zone1/slot1：base 牌（决定结果的功能牌型和子系）
//   - zone2/slot2：ingredient 牌
//   - opts：当前场地效果修正
//
// 成功时两张源牌从各自区域移除，结果牌放入合成区。
// 失败时所有状态回滚（源牌归位），调用方收到明确的错误。
func (h *HandZone) SynthesizeCards(
	zone1 string, slot1 int,
	zone2 string, slot2 int,
	opts SynthesisOpts,
) (*Card, error) {
	// 取出两张牌（失败时已经是 nil，不需要回滚）
	base, err := h.takeFromZone(zone1, slot1)
	if err != nil {
		return nil, fmt.Errorf("base 牌: %w", err)
	}

	ingredient, err := h.takeFromZone(zone2, slot2)
	if err != nil {
		// ingredient 取失败，base 要放回
		h.putBackToZone(zone1, slot1, base)
		return nil, fmt.Errorf("ingredient 牌: %w", err)
	}

	// 执行合成
	result, err := Combine(base, ingredient, opts)
	if err != nil {
		// 合成规则不允许，两张牌均放回
		h.putBackToZone(zone1, slot1, base)
		h.putBackToZone(zone2, slot2, ingredient)
		return nil, err
	}

	// 将结果放入合成区
	if err := h.PutSynth(result); err != nil {
		// 合成区满，回滚
		h.putBackToZone(zone1, slot1, base)
		h.putBackToZone(zone2, slot2, ingredient)
		return nil, fmt.Errorf("合成区已满，无法放入结果: %w", err)
	}

	return result, nil
}

// takeFromZone 从指定区域取牌，zone = "hand" | "synth"。
func (h *HandZone) takeFromZone(zone string, slot int) (*Card, error) {
	switch zone {
	case "hand":
		c, err := h.TakeHand(slot)
		if err != nil {
			return nil, err
		}
		if c == nil {
			return nil, fmt.Errorf("手牌槽位 %d 为空", slot)
		}
		return c, nil
	case "synth":
		c, err := h.TakeSynth(slot)
		if err != nil {
			return nil, err
		}
		if c == nil {
			return nil, fmt.Errorf("合成区槽位 %d 为空", slot)
		}
		return c, nil
	default:
		return nil, fmt.Errorf("未知区域: %s（应为 hand 或 synth）", zone)
	}
}

// putBackToZone 将牌放回到原来的区域和槽位（仅用于回滚）。
func (h *HandZone) putBackToZone(zone string, slot int, c *Card) {
	if c == nil {
		return
	}
	switch zone {
	case "hand":
		h.hand[slot-1] = c // 直接写回，绕过 PlaceHand 的"槽位已有牌"检查
	case "synth":
		h.synth[slot-1] = c
	}
}

// DrawIntoHand 从牌堆抽至多 n 张牌放入手牌区的空槽。
// 不会超过手牌区总容量，牌堆空时提前停止。
// 返回实际抽到的张数。
// 用途：技能/解放的"抽N张牌"效果（区别于 Fill 的"补满至N张"）。
func (h *HandZone) DrawIntoHand(deck *Deck, n int) int {
	drawn := 0
	for i := 0; i < HandZoneSize && drawn < n; i++ {
		if h.hand[i] == nil {
			c := deck.Draw()
			if c == nil {
				break // 牌堆已空
			}
			h.hand[i] = c
			drawn++
		}
	}
	return drawn
}

// ════════════════════════════════════════════════════════════════
//  阶段清场
// ════════════════════════════════════════════════════════════════

// ClearDiscardZone 清空弃牌区（手牌槽位 5-8），在清场阶段调用。
// 安全区（1-4）和合成区不受影响。
// 返回被清除的牌列表（供日志或动画展示）。
func (h *HandZone) ClearDiscardZone() []*Card {
	discarded := make([]*Card, 0, 4)
	for i := SafeZoneSize; i < HandZoneSize; i++ {
		if h.hand[i] != nil {
			discarded = append(discarded, h.hand[i])
			h.hand[i] = nil
		}
	}
	return discarded
}

// ════════════════════════════════════════════════════════════════
//  带槽位的迭代（供 BuildView 使用）
// ════════════════════════════════════════════════════════════════

// SlottedCard 是一张牌加上它所在的槽位编号（1-indexed）。
type SlottedCard struct {
	Slot int
	Card *Card
}

// HandSlottedCards 返回手牌区所有非空槽位及其牌，保留槽位顺序。
// 用于将手牌区状态序列化成协议视图。
func (h *HandZone) HandSlottedCards() []SlottedCard {
	result := make([]SlottedCard, 0, HandZoneSize)
	for i, c := range h.hand {
		if c != nil {
			result = append(result, SlottedCard{Slot: i + 1, Card: c})
		}
	}
	return result
}

// SynthSlottedCards 返回合成区所有非空槽位及其牌。
func (h *HandZone) SynthSlottedCards() []SlottedCard {
	result := make([]SlottedCard, 0, SynthZoneSize)
	for i, c := range h.synth {
		if c != nil {
			result = append(result, SlottedCard{Slot: i + 1, Card: c})
		}
	}
	return result
}

// ════════════════════════════════════════════════════════════════
//  辅助函数
// ════════════════════════════════════════════════════════════════

func validHandSlot(slot int) bool  { return slot >= 1 && slot <= HandZoneSize }
func validSynthSlot(slot int) bool { return slot >= 1 && slot <= SynthZoneSize }
