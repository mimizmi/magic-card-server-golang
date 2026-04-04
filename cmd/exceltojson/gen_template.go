//go:build ignore

// gen_template.go 生成预填数据的 Excel 模板文件（策划友好版，全中文）。
//
// 用法：go run gen_template.go
package main

import (
	"fmt"
	"log"

	"github.com/xuri/excelize/v2"
)

func main() {
	f := excelize.NewFile()
	defer f.Close()

	// 通用样式：表头加粗 + 背景色
	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 11},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"#D9E1F2"}},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
		Border: []excelize.Border{
			{Type: "bottom", Color: "#4472C4", Style: 2},
		},
	})
	descStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Color: "#808080", Italic: true, Size: 10},
		Alignment: &excelize.Alignment{WrapText: true},
	})
	noteStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Color: "#808080", Size: 9},
		Alignment: &excelize.Alignment{WrapText: true, Vertical: "top"},
	})
	// 分隔行样式（角色之间的空行）
	sepStyle, _ := f.NewStyle(&excelize.Style{
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"#F2F2F2"}},
	})

	// ══════════════════════════════════════════════════════════════
	//  Sheet 1: 角色属性
	// ══════════════════════════════════════════════════════════════
	sheet1 := "角色属性"
	f.SetSheetName("Sheet1", sheet1)

	headers1 := []string{
		"角色名称", "角色ID", "最大生命", "最大能量",
		"解放阈值", "解放方式", "被动·攻击加伤", "被动·受击减伤", "被动·濒死拦截",
	}
	for i, h := range headers1 {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet1, cell, h)
		f.SetCellStyle(sheet1, cell, cell, headerStyle)
	}

	// 第2行：填写说明
	descs1 := []string{
		"显示名称", "英文标识(勿改)", "整数", "整数",
		"0=无解放", "手动/自动/无", "整数,0=无", "整数,0=无", "是/否",
	}
	for i, d := range descs1 {
		cell, _ := excelize.CoordinatesToCellName(i+1, 2)
		f.SetCellValue(sheet1, cell, d)
		f.SetCellStyle(sheet1, cell, cell, descStyle)
	}

	charData := [][]any{
		{"力裁者", "licai", 100, 100, 80, "手动", 1, "", ""},
		{"镜换者", "jinghuan", 90, 100, 80, "手动", "", 1, ""},
		{"空手者", "kongshou", 95, 100, 60, "手动", "", "", ""},
		{"噬渊者", "shiyuan", 95, 100, 80, "手动", "", "", ""},
		{"灼血者", "zhuoxue", 85, 100, 80, "手动", 2, "", ""},
		{"殉道者", "xundao", 110, 100, 80, "自动", "", "", "是"},
		{"时空裂缝者", "liewen", 150, 150, 0, "无", "", "", ""},
		{"万能者", "wanneng", 80, 100, 0, "无", "", "", ""},
		{"血魔", "xuemo", 90, 100, 0, "无", "", "", ""},
	}
	for r, data := range charData {
		for c, val := range data {
			cell, _ := excelize.CoordinatesToCellName(c+1, r+3)
			f.SetCellValue(sheet1, cell, val)
		}
	}

	// 列宽
	f.SetColWidth(sheet1, "A", "A", 14)
	f.SetColWidth(sheet1, "B", "B", 16)
	f.SetColWidth(sheet1, "C", "F", 10)
	f.SetColWidth(sheet1, "G", "I", 14)

	// 冻结首行+说明行
	f.SetPanes(sheet1, &excelize.Panes{
		Freeze:      true,
		Split:       false,
		XSplit:      0,
		YSplit:      2,
		TopLeftCell: "A3",
		ActivePane:  "bottomLeft",
	})

	// ══════════════════════════════════════════════════════════════
	//  Sheet 2: 角色技能
	// ══════════════════════════════════════════════════════════════
	sheet2 := "角色技能"
	f.NewSheet(sheet2)

	headers2 := []string{
		"角色名称", "技能等级", "技能名称", "能量消耗",
		"造成伤害", "回复生命", "获得能量", "摸牌数", "自伤", "技能描述",
	}
	for i, h := range headers2 {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet2, cell, h)
		f.SetCellStyle(sheet2, cell, cell, headerStyle)
	}

	descs2 := []string{
		"与【角色属性】一致", "普通/强化/解放", "技能显示名", "整数",
		"整数,0=无", "整数,0=无", "整数,0=无", "整数,0=无", "整数,0=无", "技能说明文字",
	}
	for i, d := range descs2 {
		cell, _ := excelize.CoordinatesToCellName(i+1, 2)
		f.SetCellValue(sheet2, cell, d)
		f.SetCellStyle(sheet2, cell, cell, descStyle)
	}

	// 技能数据：每角色3行（普通/强化/解放），角色间空1行分隔
	type skillRow struct {
		charName string
		level    string
		name     string
		cost     any
		damage   any
		heal     any
		energy   any
		draw     any
		selfDmg  any
		desc     string
	}

	skillData := []any{
		// 力裁者
		skillRow{"力裁者", "普通", "力裁斩击", 10, 8, "", "", "", "", "力裁斩击：对对手造成8点直接伤害"},
		skillRow{"力裁者", "强化", "强化裁决", 20, 16, "", "", "", "", "强化裁决：对对手造成16点直接伤害"},
		skillRow{"力裁者", "解放", "绝对裁决", 80, 30, 20, "", "", "", "绝对裁决：造成30点直接伤害并回复20点生命"},
		"sep",
		// 镜换者
		skillRow{"镜换者", "普通", "镜像映射", 8, "", 5, "", 2, "", "镜像映射：摸2张牌并回复5点生命"},
		skillRow{"镜换者", "强化", "镜像反击", 16, 10, "", "", 3, "", "镜像反击：摸3张牌并造成10点直接伤害"},
		skillRow{"镜换者", "解放", "镜换轮转", 80, 20, 20, "", 2, "", "镜换轮转：造成20点直接伤害，回复20点生命，摸2张牌"},
		"sep",
		// 空手者
		skillRow{"空手者", "普通", "虚拳引气", 5, "", "", 20, "", "", "虚拳引气：获得20点能量"},
		skillRow{"空手者", "强化", "引气冲拳", 10, 8, "", 30, "", "", "引气冲拳：获得30点能量并造成8点直接伤害"},
		skillRow{"空手者", "解放", "空手相搏", 60, 20, 20, 20, "", "", "空手相搏：造成20点直接伤害，回复20点生命，获得20点能量"},
		"sep",
		// 噬渊者
		skillRow{"噬渊者", "普通", "噬渊之触", 10, 6, 6, "", "", "", "噬渊之触：造成6点直接伤害并汲取6点生命"},
		skillRow{"噬渊者", "强化", "深渊噬魂", 20, 14, 10, "", "", "", "深渊噬魂：造成14点直接伤害并汲取10点生命"},
		skillRow{"噬渊者", "解放", "渊噬万物", 80, 28, 20, "", "", "", "渊噬万物：造成28点直接伤害并汲取20点生命"},
		"sep",
		// 灼血者
		skillRow{"灼血者", "普通", "灼血冲击", 10, 10, "", "", "", "", "灼血冲击：造成10点直接伤害"},
		skillRow{"灼血者", "强化", "烈焰灼血", 20, 20, "", "", "", "", "烈焰灼血：造成20点直接伤害"},
		skillRow{"灼血者", "解放", "血焰爆发", 80, 40, "", "", "", "", "血焰爆发：造成40点直接伤害"},
		"sep",
		// 殉道者
		skillRow{"殉道者", "普通", "殉道之愿", 8, "", 10, "", "", "", "殉道之愿：回复10点生命"},
		skillRow{"殉道者", "强化", "殉道之力", 16, "", 20, 10, "", "", "殉道之力：回复20点生命并获得10点能量"},
		skillRow{"殉道者", "解放", "殉道解放", 0, "", 30, 10, "", "", "殉道解放：自动触发，濒死时回复30点生命并获得10点能量"},
		"sep",
		// 时空裂缝者 — 无普通技能，由特殊机制控制
		"sep",
		// 万能者 — 无普通技能，由特殊机制控制
		"sep",
		// 血魔 — 无普通技能，由特殊机制控制
	}

	row := 3
	for _, item := range skillData {
		if s, ok := item.(string); ok && s == "sep" {
			// 分隔行
			for c := 1; c <= 10; c++ {
				cell, _ := excelize.CoordinatesToCellName(c, row)
				f.SetCellStyle(sheet2, cell, cell, sepStyle)
			}
			row++
			continue
		}
		sk := item.(skillRow)
		vals := []any{sk.charName, sk.level, sk.name, sk.cost, sk.damage, sk.heal, sk.energy, sk.draw, sk.selfDmg, sk.desc}
		for c, val := range vals {
			cell, _ := excelize.CoordinatesToCellName(c+1, row)
			f.SetCellValue(sheet2, cell, val)
		}
		row++
	}

	f.SetColWidth(sheet2, "A", "A", 14)
	f.SetColWidth(sheet2, "B", "B", 10)
	f.SetColWidth(sheet2, "C", "C", 14)
	f.SetColWidth(sheet2, "D", "I", 10)
	f.SetColWidth(sheet2, "J", "J", 50)

	f.SetPanes(sheet2, &excelize.Panes{
		Freeze: true, XSplit: 0, YSplit: 2, TopLeftCell: "A3", ActivePane: "bottomLeft",
	})

	// ══════════════════════════════════════════════════════════════
	//  Sheet 3: 特殊机制
	// ══════════════════════════════════════════════════════════════
	sheet3 := "特殊机制"
	f.NewSheet(sheet3)

	headers3 := []string{"角色名称", "参数名称", "参数值", "说明（仅供参考，无需填写）"}
	for i, h := range headers3 {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet3, cell, h)
		f.SetCellStyle(sheet3, cell, cell, headerStyle)
	}

	type paramRow struct {
		char  string
		param string
		value any
		note  string
	}

	paramData := []any{
		// ── 时空裂缝者 ──
		paramRow{"时空裂缝者", "HP与能量共享", "是", "是=HP和能量共用一个数值池"},
		paramRow{"时空裂缝者", "初始生命值", 60, "开局实际HP（而非最大值）"},
		paramRow{"时空裂缝者", "初始能量值", 60, "开局实际能量"},
		paramRow{"时空裂缝者", "裂缝基础能量", 3, "每个裂缝提供的基础能量加成"},
		paramRow{"时空裂缝者", "裂缝能量递增", 2, "每多一个裂缝，加成额外增加的量"},
		paramRow{"时空裂缝者", "普通技能消耗", 15, "开启裂缝消耗的能量"},
		paramRow{"时空裂缝者", "强化技能消耗", 30, "强化裂缝消耗的能量"},
		paramRow{"时空裂缝者", "强化技能点数阈值", 3, "需要多少合成点才能使用强化技能"},
		paramRow{"时空裂缝者", "超能解放能量阈值", 100, "能量达到此值时触发超能解放"},
		"sep",
		// ── 万能者 ──
		paramRow{"万能者", "全牌攻击化", "是", "是=所有卡牌都视为攻击牌"},
		paramRow{"万能者", "阶段伤害阈值", "10,50,100", "逗号分隔，依次为阶段1/2/3的伤害阈值"},
		paramRow{"万能者", "阶段1攻击加成", 2, "阶段1时每次攻击额外伤害"},
		paramRow{"万能者", "阶段2牌面加成", 2, "阶段2时出牌点数加成"},
		paramRow{"万能者", "阶段3攻击倍率", 2, "阶段3时攻击伤害倍率"},
		"sep",
		// ── 血魔 ──
		paramRow{"血魔", "受伤牌面加成阈值", 50, "累计受伤达到此值后，牌面获得加成"},
		paramRow{"血魔", "受伤后牌面加成", 3, "触发后每张牌额外加成点数"},
		paramRow{"血魔", "吸血伤害阈值", 30, "累计造成伤害达到此值后可激活吸血"},
		paramRow{"血魔", "吸血激活点数", 25, "激活吸血需要的合成点数"},
		paramRow{"血魔", "强化技能点数阈值", 3, "使用强化技能需要的合成点数"},
		paramRow{"血魔", "强化技能自伤", 20, "使用强化技能时自伤的HP"},
		paramRow{"血魔", "强化技能攻击加成", 10, "使用强化技能时额外攻击伤害"},
		paramRow{"血魔", "普通技能自伤", 10, "使用普通技能时自伤的HP"},
		paramRow{"血魔", "普通技能摸牌数", 2, "使用普通技能时摸牌张数"},
	}

	row = 2
	for _, item := range paramData {
		if s, ok := item.(string); ok && s == "sep" {
			for c := 1; c <= 4; c++ {
				cell, _ := excelize.CoordinatesToCellName(c, row)
				f.SetCellStyle(sheet2, cell, cell, sepStyle)
			}
			row++
			continue
		}
		p := item.(paramRow)
		vals := []any{p.char, p.param, p.value, p.note}
		for c, val := range vals {
			cell, _ := excelize.CoordinatesToCellName(c+1, row)
			f.SetCellValue(sheet3, cell, val)
		}
		// 说明列灰色
		noteCell, _ := excelize.CoordinatesToCellName(4, row)
		f.SetCellStyle(sheet3, noteCell, noteCell, noteStyle)
		row++
	}

	f.SetColWidth(sheet3, "A", "A", 14)
	f.SetColWidth(sheet3, "B", "B", 20)
	f.SetColWidth(sheet3, "C", "C", 16)
	f.SetColWidth(sheet3, "D", "D", 40)

	f.SetPanes(sheet3, &excelize.Panes{
		Freeze: true, XSplit: 0, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft",
	})

	// ══════════════════════════════════════════════════════════════
	//  Sheet 4: 场地效果
	// ══════════════════════════════════════════════════════════════
	sheet4 := "场地效果"
	f.NewSheet(sheet4)

	headers4 := []string{
		"场地名称", "场地ID", "虚幻加成", "允许同系合成",
		"轮回规则", "隐藏摸牌", "攻击加成", "濒死吸取",
	}
	for i, h := range headers4 {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet4, cell, h)
		f.SetCellStyle(sheet4, cell, cell, headerStyle)
	}

	descs4 := []string{
		"显示名称", "英文标识(勿改)", "是/否", "是/否",
		"0=无/1=基础/2=变体", "是/否", "整数,0=无", "整数,0=无",
	}
	for i, d := range descs4 {
		cell, _ := excelize.CoordinatesToCellName(i+1, 2)
		f.SetCellValue(sheet4, cell, d)
		f.SetCellStyle(sheet4, cell, cell, descStyle)
	}

	fieldData := [][]any{
		{"空旷之地", "clear", "", "", 0, "", "", ""},
		{"虚幻之境·实", "illusion_real", "是", "", 0, "", "", ""},
		{"虚幻之境·虚", "illusion_void", "", "", 0, "是", "", ""},
		{"轮回之境·实", "reinc_base", "", "", 1, "", "", ""},
		{"轮回之境·虚", "reinc_other", "", "", 2, "", "", ""},
		{"混沌之域", "chaos", "", "是", 0, "", "", ""},
		{"回响之地", "echo", "", "", 0, "", 1, ""},
		{"守护之光", "protect", "", "", 0, "", "", 15},
	}
	for r, data := range fieldData {
		for c, val := range data {
			cell, _ := excelize.CoordinatesToCellName(c+1, r+3)
			f.SetCellValue(sheet4, cell, val)
		}
	}

	f.SetColWidth(sheet4, "A", "A", 16)
	f.SetColWidth(sheet4, "B", "B", 16)
	f.SetColWidth(sheet4, "C", "H", 14)

	f.SetPanes(sheet4, &excelize.Panes{
		Freeze: true, XSplit: 0, YSplit: 2, TopLeftCell: "A3", ActivePane: "bottomLeft",
	})

	// 保存
	outPath := "data/游戏配置表.xlsx"
	if err := f.SaveAs(outPath); err != nil {
		log.Fatalf("保存失败: %v", err)
	}
	fmt.Printf("✓ 模板已生成 → %s\n", outPath)
}
