package character

// chars.go 注册全部 6 个角色。
//
// 角色设计思路：
//   - 力裁者：纯攻击，靠直接伤害技能拿下对手
//   - 镜换者：防守反击，减免被动 + 摸牌型技能
//   - 空手者：能量流，技能获取大量能量，靠解放爆发
//   - 噬渊者：生命汲取，每次技能都自己回血
//   - 灼血者：高风险高回报，攻击加成最高但 HP 最低
//   - 殉道者：濒死专精，被动能拦截一次二次死亡并自动解放

func init() {
	register(&CharDef{
		ID:           "licai",
		Name:         "力裁者",
		MaxHP:        100,
		MaxEnergy:    100,
		LibThreshold: 80,
		ManualLib:    true,
		Passive: PassiveTraits{
			BonusOutgoing: 1, // 攻击牌额外 +1 伤害
		},
		Normal: SkillDef{
			Name:       "力裁斩击",
			EnergyCost: 10,
			Result: SkillResult{
				Tier:             TierNormal,
				DealDirectDamage: 8,
				Desc:             "力裁斩击：对对手造成8点直接伤害",
			},
		},
		Enhanced: SkillDef{
			Name:       "强化裁决",
			EnergyCost: 20,
			Result: SkillResult{
				Tier:             TierEnhanced,
				DealDirectDamage: 16,
				Desc:             "强化裁决：对对手造成16点直接伤害",
			},
		},
		Lib: SkillDef{
			Name:       "绝对裁决",
			EnergyCost: 80,
			Result: SkillResult{
				Tier:             TierLiberation,
				DealDirectDamage: 30,
				HealSelf:         20,
				Desc:             "绝对裁决：造成30点直接伤害并回复20点生命",
			},
		},
	})

	register(&CharDef{
		ID:           "jinghuan",
		Name:         "镜换者",
		MaxHP:        90,
		MaxEnergy:    100,
		LibThreshold: 80,
		ManualLib:    true,
		Passive: PassiveTraits{
			IncomingReduction: 1, // 每次受到的伤害 -1
		},
		Normal: SkillDef{
			Name:       "镜像映射",
			EnergyCost: 8,
			Result: SkillResult{
				Tier:      TierNormal,
				DrawCards: 2,
				HealSelf:  5,
				Desc:      "镜像映射：摸2张牌并回复5点生命",
			},
		},
		Enhanced: SkillDef{
			Name:       "镜像反击",
			EnergyCost: 16,
			Result: SkillResult{
				Tier:             TierEnhanced,
				DrawCards:        3,
				DealDirectDamage: 10,
				Desc:             "镜像反击：摸3张牌并造成10点直接伤害",
			},
		},
		Lib: SkillDef{
			Name:       "镜换轮转",
			EnergyCost: 80,
			Result: SkillResult{
				Tier:             TierLiberation,
				DealDirectDamage: 20,
				HealSelf:         20,
				DrawCards:        2,
				Desc:             "镜换轮转：造成20点直接伤害，回复20点生命，摸2张牌",
			},
		},
	})

	register(&CharDef{
		ID:           "kongshou",
		Name:         "空手者",
		MaxHP:        95,
		MaxEnergy:    100,
		LibThreshold: 60,
		ManualLib:    true,
		Passive:      PassiveTraits{}, // 无被动，靠技能补偿
		Normal: SkillDef{
			Name:       "虚拳引气",
			EnergyCost: 5,
			Result: SkillResult{
				Tier:       TierNormal,
				GainEnergy: 20,
				Desc:       "虚拳引气：获得20点能量",
			},
		},
		Enhanced: SkillDef{
			Name:       "引气冲拳",
			EnergyCost: 10,
			Result: SkillResult{
				Tier:             TierEnhanced,
				GainEnergy:       30,
				DealDirectDamage: 8,
				Desc:             "引气冲拳：获得30点能量并造成8点直接伤害",
			},
		},
		Lib: SkillDef{
			Name:       "空手相搏",
			EnergyCost: 60,
			Result: SkillResult{
				Tier:             TierLiberation,
				DealDirectDamage: 20,
				HealSelf:         20,
				GainEnergy:       20,
				Desc:             "空手相搏：造成20点直接伤害，回复20点生命，获得20点能量",
			},
		},
	})

	register(&CharDef{
		ID:           "shiyuan",
		Name:         "噬渊者",
		MaxHP:        95,
		MaxEnergy:    100,
		LibThreshold: 80,
		ManualLib:    true,
		Passive:      PassiveTraits{}, // 无被动，技能内置汲取
		Normal: SkillDef{
			Name:       "噬渊之触",
			EnergyCost: 10,
			Result: SkillResult{
				Tier:             TierNormal,
				DealDirectDamage: 6,
				HealSelf:         6,
				Desc:             "噬渊之触：造成6点直接伤害并汲取6点生命",
			},
		},
		Enhanced: SkillDef{
			Name:       "深渊噬魂",
			EnergyCost: 20,
			Result: SkillResult{
				Tier:             TierEnhanced,
				DealDirectDamage: 14,
				HealSelf:         10,
				Desc:             "深渊噬魂：造成14点直接伤害并汲取10点生命",
			},
		},
		Lib: SkillDef{
			Name:       "渊噬万物",
			EnergyCost: 80,
			Result: SkillResult{
				Tier:             TierLiberation,
				DealDirectDamage: 28,
				HealSelf:         20,
				Desc:             "渊噬万物：造成28点直接伤害并汲取20点生命",
			},
		},
	})

	register(&CharDef{
		ID:           "zhuoxue",
		Name:         "灼血者",
		MaxHP:        85,
		MaxEnergy:    100,
		LibThreshold: 80,
		ManualLib:    true,
		Passive: PassiveTraits{
			BonusOutgoing: 2, // 攻击牌额外 +2 伤害（最高加成）
		},
		Normal: SkillDef{
			Name:       "灼血冲击",
			EnergyCost: 10,
			Result: SkillResult{
				Tier:             TierNormal,
				DealDirectDamage: 10,
				Desc:             "灼血冲击：造成10点直接伤害",
			},
		},
		Enhanced: SkillDef{
			Name:       "烈焰灼血",
			EnergyCost: 20,
			Result: SkillResult{
				Tier:             TierEnhanced,
				DealDirectDamage: 20,
				Desc:             "烈焰灼血：造成20点直接伤害",
			},
		},
		Lib: SkillDef{
			Name:       "血焰爆发",
			EnergyCost: 80,
			Result: SkillResult{
				Tier:             TierLiberation,
				DealDirectDamage: 40,
				Desc:             "血焰爆发：造成40点直接伤害",
			},
		},
	})

	register(&CharDef{
		ID:           "xundao",
		Name:         "殉道者",
		MaxHP:        110,
		MaxEnergy:    100,
		LibThreshold: 80,
		ManualLib:    false, // 解放由引擎在二次死亡时自动触发
		Passive: PassiveTraits{
			InterceptNearDeath: true, // 被动：在二次死亡时自动触发解放（每局一次）
		},
		Normal: SkillDef{
			Name:       "殉道之愿",
			EnergyCost: 8,
			Result: SkillResult{
				Tier:     TierNormal,
				HealSelf: 10,
				Desc:     "殉道之愿：回复10点生命",
			},
		},
		Enhanced: SkillDef{
			Name:       "殉道之力",
			EnergyCost: 16,
			Result: SkillResult{
				Tier:       TierEnhanced,
				HealSelf:   20,
				GainEnergy: 10,
				Desc:       "殉道之力：回复20点生命并获得10点能量",
			},
		},
		Lib: SkillDef{
			Name:       "殉道解放",
			EnergyCost: 0, // 自动触发，不消耗能量
			Result: SkillResult{
				Tier:     TierLiberation,
				HealSelf: 30, // 引擎另外将 HP 设为60，此处额外回复由 applySkillResult 处理
				Desc:     "殉道解放：自动触发，濒死时回复30点生命并获得10点能量",
				GainEnergy: 10,
			},
		},
	})
}
