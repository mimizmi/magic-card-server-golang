package character_test

import (
	"testing"

	"echo/internal/game/character"
)

// TestXuemoSkillCostZero 血魔者所有技能档位的能量消耗必须为 0。
func TestXuemoSkillCostZero(t *testing.T) {
	inst, err := character.NewInstance("momiji")
	if err != nil {
		t.Fatalf("NewInstance: %v", err)
	}

	cases := []struct {
		pts   int
		label string
	}{
		{1, "普通技能(pts=1)"},
		{3, "强化技能(pts=3)"},
		{25, "吸血激活(pts=25)"},
	}

	for _, tc := range cases {
		_, cost, err := inst.UseSkill(tc.pts)
		if err != nil {
			t.Errorf("%s: UseSkill error: %v", tc.label, err)
			continue
		}
		if cost != 0 {
			t.Errorf("%s: cost = %d, want 0", tc.label, cost)
		}
	}
}

// TestLiewenPhaseStart_OnlyActionPhase 裂缝能量只在 "action" 阶段开始时产生。
func TestLiewenPhaseStart_OnlyActionPhase(t *testing.T) {
	inst, err := character.NewInstance("shigure")
	if err != nil {
		t.Fatalf("NewInstance: %v", err)
	}

	// 先设置裂缝状态
	inst.ExtraState["rifts"] = 2
	inst.ExtraState["rift_bonus"] = 3

	nonActionPhases := []string{"field_draw", "draw", "combat", "cleanup"}
	for _, phase := range nonActionPhases {
		delta, msg := inst.Def.Hooks.OnPhaseStart(phase, inst.ExtraState)
		if delta != 0 {
			t.Errorf("phase=%s: delta = %d, want 0", phase, delta)
		}
		if msg != "" {
			t.Errorf("phase=%s: msg = %q, want empty", phase, msg)
		}
	}

	// action 阶段应产生 rifts*rift_bonus = 6 点
	delta, msg := inst.Def.Hooks.OnPhaseStart("action", inst.ExtraState)
	if delta != 6 {
		t.Errorf("action phase: delta = %d, want 6 (2 rifts × 3 bonus)", delta)
	}
	if msg == "" {
		t.Error("action phase: expected non-empty msg")
	}
}
