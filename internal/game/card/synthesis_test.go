package card_test

// synthesis_test.go — 合成系统的表驱动单元测试
//
// 学习要点：
//   1. Go 表驱动测试（table-driven tests）的标准写法
//   2. 测试纯函数：相同输入一定给出相同输出，最易验证
//   3. 测试错误路径（sentinel error 比较）
//   4. 测试边界条件（点数上限截断）

import (
	"errors"
	"testing"

	"echo/internal/game/card"
)

// ════════════════════════════════════════════════════════════════
//  辅助工厂
// ════════════════════════════════════════════════════════════════

// atk / skl / eng 快速创建指定子系、指定牌型、指定点数的牌。
func atk(sf card.SubFaction, pts int) *card.Card { return card.New(sf, card.TypeAttack, pts) }
func skl(sf card.SubFaction, pts int) *card.Card { return card.New(sf, card.TypeSkill, pts) }
func eng(sf card.SubFaction, pts int) *card.Card { return card.New(sf, card.TypeEnergy, pts) }

// ════════════════════════════════════════════════════════════════
//  Combine 主规则测试
// ════════════════════════════════════════════════════════════════

func TestCombine_SameMajor_Multiplies(t *testing.T) {
	// 同大系（梦幻+梦幻）→ 点数相乘
	cases := []struct {
		name   string
		base   *card.Card
		ingr   *card.Card
		want   int
	}{
		{"梦境攻击×梦境技能 2×3=6>5截断", atk(card.SubDream, 2), skl(card.SubDream, 3), 5},
		{"梦境攻击×梦境技能 2×2=4", atk(card.SubDream, 2), skl(card.SubDream, 2), 4},
		{"虚幻攻击×虚幻能耗 1×5=5", atk(card.SubIllusion, 1), eng(card.SubIllusion, 5), 5},
		{"重组攻击×重组技能 3×2=6>5截断", atk(card.SubReform, 3), skl(card.SubReform, 2), 5},
	}

	for _, tc := range cases {
		tc := tc // 防止闭包捕获问题（Go 1.22 前需要）
		t.Run(tc.name, func(t *testing.T) {
			result, err := card.Combine(tc.base, tc.ingr, card.DefaultOpts())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Points != tc.want {
				t.Errorf("points = %d, want %d", result.Points, tc.want)
			}
			// 结果继承 base 的属性
			if result.CardType != tc.base.CardType {
				t.Errorf("CardType = %v, want %v", result.CardType, tc.base.CardType)
			}
			if result.SubFaction != tc.base.SubFaction {
				t.Errorf("SubFaction = %v, want %v", result.SubFaction, tc.base.SubFaction)
			}
		})
	}
}

func TestCombine_DifferentMajor_Adds(t *testing.T) {
	// 不同大系（梦幻+重回）→ 点数相加
	cases := []struct {
		name string
		base *card.Card
		ingr *card.Card
		want int
	}{
		{"梦境攻击+轮回技能 2+3=5", atk(card.SubDream, 2), skl(card.SubReincarnation, 3), 5},
		{"虚幻攻击+重组技能 3+4=7>5截断", atk(card.SubIllusion, 3), skl(card.SubReform, 4), 5},
		{"梦境技能+重组攻击 1+1=2", skl(card.SubDream, 1), atk(card.SubReform, 1), 2},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			result, err := card.Combine(tc.base, tc.ingr, card.DefaultOpts())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Points != tc.want {
				t.Errorf("points = %d, want %d", result.Points, tc.want)
			}
		})
	}
}

// ════════════════════════════════════════════════════════════════
//  错误路径：同类型禁止
// ════════════════════════════════════════════════════════════════

func TestCombine_SameType_ReturnsErrSameCardType(t *testing.T) {
	pairs := [][2]*card.Card{
		{atk(card.SubDream, 1), atk(card.SubReform, 2)},       // 攻击+攻击
		{skl(card.SubDream, 1), skl(card.SubDream, 2)},        // 技能+技能
		{eng(card.SubIllusion, 3), eng(card.SubReincarnation, 1)}, // 能耗+能耗
	}

	for _, pair := range pairs {
		_, err := card.Combine(pair[0], pair[1], card.DefaultOpts())
		if !errors.Is(err, card.ErrSameCardType) {
			t.Errorf("Combine(%v, %v) = %v, want ErrSameCardType", pair[0], pair[1], err)
		}
	}
}

// ════════════════════════════════════════════════════════════════
//  场地效果：混沌之域 AllowSameType
// ════════════════════════════════════════════════════════════════

func TestCombine_AllowSameType_NoError(t *testing.T) {
	opts := card.DefaultOpts()
	opts.AllowSameType = true

	base := atk(card.SubDream, 2)
	ingr := atk(card.SubReform, 3) // 不同大系，相加 = 5

	result, err := card.Combine(base, ingr, opts)
	if err != nil {
		t.Fatalf("unexpected error with AllowSameType: %v", err)
	}
	// 相加：梦幻+重回 → 2+3=5
	if result.Points != 5 {
		t.Errorf("points = %d, want 5", result.Points)
	}
}

func TestCombine_AllowSameType_SameMajorMultiplies(t *testing.T) {
	opts := card.DefaultOpts()
	opts.AllowSameType = true

	// 同大系同类型：2×3=6 → 截断到5
	base := atk(card.SubDream, 2)
	ingr := atk(card.SubIllusion, 3)

	result, err := card.Combine(base, ingr, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Points != 5 {
		t.Errorf("points = %d, want 5", result.Points)
	}
}

// ════════════════════════════════════════════════════════════════
//  场地效果：虚幻之境·实 IllusionBonus
// ════════════════════════════════════════════════════════════════

func TestCombine_IllusionBonus_CapAt7ForIllusion(t *testing.T) {
	opts := card.DefaultOpts()
	opts.IllusionBonus = true

	// base 是虚幻牌，同大系乘法：3×3=9 → 上限提升至7
	base := atk(card.SubIllusion, 3)
	ingr := skl(card.SubIllusion, 3)

	result, err := card.Combine(base, ingr, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Points != 7 {
		t.Errorf("IllusionBonus: points = %d, want 7", result.Points)
	}
}

func TestCombine_IllusionBonus_NoCapForNonIllusion(t *testing.T) {
	opts := card.DefaultOpts()
	opts.IllusionBonus = true

	// base 是梦境牌（非虚幻），上限仍为5：3×3=9 → 5
	base := atk(card.SubDream, 3)
	ingr := skl(card.SubDream, 3)

	result, err := card.Combine(base, ingr, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Points != 5 {
		t.Errorf("non-illusion base: points = %d, want 5", result.Points)
	}
}

// ════════════════════════════════════════════════════════════════
//  场地效果：轮回之境·实 ReincarnationAsBase
// ════════════════════════════════════════════════════════════════

func TestCombine_ReincarnAsBase_UsesReincarnPoints(t *testing.T) {
	opts := card.DefaultOpts()
	opts.ReincarnationRule = card.ReincarnationAsBase

	// 轮回牌点数4，另一张点数2 → 结果 = 轮回牌自身 = 4
	base := atk(card.SubReincarnation, 4)
	ingr := skl(card.SubDream, 2)

	result, err := card.Combine(base, ingr, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Points != 4 {
		t.Errorf("ReincarnAsBase: points = %d, want 4", result.Points)
	}
}

func TestCombine_ReincarnAsBase_IngredientIsReinc(t *testing.T) {
	opts := card.DefaultOpts()
	opts.ReincarnationRule = card.ReincarnationAsBase

	// ingredient 是轮回牌
	base := atk(card.SubDream, 3)
	ingr := skl(card.SubReincarnation, 2)

	result, err := card.Combine(base, ingr, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Points != 2 { // 轮回牌自身点数2
		t.Errorf("ReincarnAsBase (ingr): points = %d, want 2", result.Points)
	}
}

// ════════════════════════════════════════════════════════════════
//  场地效果：轮回之境·虚 ReincarnationAsOther
// ════════════════════════════════════════════════════════════════

func TestCombine_ReincarnAsOther_UsesOtherPoints(t *testing.T) {
	opts := card.DefaultOpts()
	opts.ReincarnationRule = card.ReincarnationAsOther

	// 轮回牌4 + 梦境牌3 → 结果 = 梦境牌 = 3
	base := atk(card.SubReincarnation, 4)
	ingr := skl(card.SubDream, 3)

	result, err := card.Combine(base, ingr, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Points != 3 {
		t.Errorf("ReincarnAsOther: points = %d, want 3", result.Points)
	}
}

// ════════════════════════════════════════════════════════════════
//  Validate 单独测试
// ════════════════════════════════════════════════════════════════

func TestValidate_NilCard(t *testing.T) {
	if err := card.Validate(nil, atk(card.SubDream, 1)); err == nil {
		t.Error("expected error for nil base")
	}
	if err := card.Validate(atk(card.SubDream, 1), nil); err == nil {
		t.Error("expected error for nil ingredient")
	}
}

func TestValidate_DifferentTypes_NoError(t *testing.T) {
	pairs := [][2]*card.Card{
		{atk(card.SubDream, 1), skl(card.SubDream, 1)},
		{atk(card.SubDream, 1), eng(card.SubReform, 1)},
		{skl(card.SubIllusion, 2), eng(card.SubReincarnation, 3)},
	}
	for _, pair := range pairs {
		if err := card.Validate(pair[0], pair[1]); err != nil {
			t.Errorf("Validate(%v, %v) unexpected error: %v", pair[0], pair[1], err)
		}
	}
}
