// cmd/jsontoexcel 是 cmd/exceltojson 的反向工具：
// 把开发者维护的 characters.json / fields.json 同步回策划友好的 Excel 配置表。
//
// 典型场景：开发者新加了一个角色（写完 hooks_<id>.go 与 characters.json 条目后），
// 跑一次本工具，把 Excel 同步成最新状态再交还给策划继续调参。
//
// 用法：
//
//	go run ./cmd/jsontoexcel \
//	    -c data/characters.json \
//	    -f data/fields.json \
//	    -o data/游戏配置表.xlsx
//
// 视觉风格（表头加粗 / 列宽 / 冻结窗格 / 中文标签）与 exceltojson 接受的输入完全对齐，
// 因此本工具产出的文件可被 exceltojson 无损读回。中文↔英文映射统一在
// echo/internal/excelmap 维护，两端共享一份事实。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"

	"echo/internal/excelmap"

	"github.com/xuri/excelize/v2"
)

// ────────────────────────────────────────────────────────────────
//  JSON 数据结构（与 internal/game/character/loader.go 保持一致）
// ────────────────────────────────────────────────────────────────

type charJSON struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	MaxHP        int            `json:"max_hp"`
	MaxEnergy    int            `json:"max_energy"`
	LibThreshold int            `json:"lib_threshold"`
	ManualLib    bool           `json:"manual_lib"`
	Passive      passiveJSON    `json:"passive"`
	Normal       skillJSON      `json:"normal"`
	Enhanced     skillJSON      `json:"enhanced"`
	Lib          skillJSON      `json:"lib"`
	HooksConfig  map[string]any `json:"hooks_config,omitempty"`
}

type passiveJSON struct {
	BonusOutgoing      int  `json:"bonus_outgoing,omitempty"`
	IncomingReduction  int  `json:"incoming_reduction,omitempty"`
	InterceptNearDeath bool `json:"intercept_near_death,omitempty"`
}

type skillJSON struct {
	Name       string     `json:"name,omitempty"`
	EnergyCost int        `json:"energy_cost,omitempty"`
	Result     resultJSON `json:"result,omitempty"`
}

type resultJSON struct {
	DealDirectDamage int    `json:"deal_direct_damage,omitempty"`
	HealSelf         int    `json:"heal_self,omitempty"`
	GainEnergy       int    `json:"gain_energy,omitempty"`
	DrawCards        int    `json:"draw_cards,omitempty"`
	DamageSelf       int    `json:"damage_self,omitempty"`
	Desc             string `json:"desc,omitempty"`
}

type fieldJSON struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	IllusionBonus  bool   `json:"illusion_bonus,omitempty"`
	AllowSameType  bool   `json:"allow_same_type,omitempty"`
	ReincarnRule   int    `json:"reincarn_rule,omitempty"`
	HideDrawnCards bool   `json:"hide_drawn_cards,omitempty"`
	BonusAttack    int    `json:"bonus_attack,omitempty"`
	NearDeathDrain int    `json:"near_death_drain,omitempty"`
}

// ────────────────────────────────────────────────────────────────
//  入口
// ────────────────────────────────────────────────────────────────

func main() {
	charsIn := flag.String("c", "data/characters.json", "characters.json 输入路径")
	fieldsIn := flag.String("f", "data/fields.json", "fields.json 输入路径")
	out := flag.String("o", "data/游戏配置表.xlsx", "Excel 输出路径")
	flag.Parse()

	chars, err := readChars(*charsIn)
	if err != nil {
		log.Fatalf("读取 %s 失败: %v", *charsIn, err)
	}
	fields, err := readFields(*fieldsIn)
	if err != nil {
		log.Fatalf("读取 %s 失败: %v", *fieldsIn, err)
	}

	if err := writeExcel(*out, chars, fields); err != nil {
		log.Fatalf("生成 Excel 失败: %v", err)
	}
	fmt.Printf("✓ 已生成 %s（%d 个角色 / %d 个场地）\n", *out, len(chars), len(fields))
}

// ────────────────────────────────────────────────────────────────
//  读 JSON
// ────────────────────────────────────────────────────────────────

func readChars(path string) ([]charJSON, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var arr []charJSON
	if err := json.Unmarshal(data, &arr); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return arr, nil
}

func readFields(path string) ([]fieldJSON, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var arr []fieldJSON
	if err := json.Unmarshal(data, &arr); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return arr, nil
}

// orderChars 把角色按 excelmap.CharOrder 排序，未列入的按 JSON 原顺序追加在末尾。
func orderChars(chars []charJSON) []charJSON {
	priority := make(map[string]int, len(excelmap.CharOrder))
	for i, name := range excelmap.CharOrder {
		priority[name] = i
	}
	const tail = 1 << 30
	type entry struct {
		c   charJSON
		ord int
		seq int
	}
	indexed := make([]entry, len(chars))
	for i, c := range chars {
		ord, ok := priority[c.Name]
		if !ok {
			ord = tail + i
		}
		indexed[i] = entry{c: c, ord: ord, seq: i}
	}
	sort.SliceStable(indexed, func(i, j int) bool {
		if indexed[i].ord != indexed[j].ord {
			return indexed[i].ord < indexed[j].ord
		}
		return indexed[i].seq < indexed[j].seq
	})
	out := make([]charJSON, len(indexed))
	for i, it := range indexed {
		out[i] = it.c
	}
	return out
}

// ────────────────────────────────────────────────────────────────
//  写 Excel
// ────────────────────────────────────────────────────────────────

func writeExcel(path string, chars []charJSON, fields []fieldJSON) error {
	chars = orderChars(chars)

	f := excelize.NewFile()
	defer f.Close()

	sty, err := buildStyles(f)
	if err != nil {
		return err
	}

	if err := writeSheetCharAttrs(f, sty, chars); err != nil {
		return err
	}
	if err := writeSheetCharSkills(f, sty, chars); err != nil {
		return err
	}
	if err := writeSheetHooks(f, sty, chars); err != nil {
		return err
	}
	if err := writeSheetFields(f, sty, fields); err != nil {
		return err
	}
	return f.SaveAs(path)
}

// styles 集中所有样式，避免重复创建。
type styles struct {
	header int
	desc   int
	sep    int
	note   int
}

func buildStyles(f *excelize.File) (*styles, error) {
	var s styles
	var err error
	s.header, err = f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 11},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"#D9E1F2"}},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
		Border: []excelize.Border{
			{Type: "bottom", Color: "#4472C4", Style: 2},
		},
	})
	if err != nil {
		return nil, err
	}
	s.desc, err = f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Color: "#808080", Italic: true, Size: 10},
		Alignment: &excelize.Alignment{WrapText: true},
	})
	if err != nil {
		return nil, err
	}
	s.sep, err = f.NewStyle(&excelize.Style{
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"#F2F2F2"}},
	})
	if err != nil {
		return nil, err
	}
	s.note, err = f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Color: "#808080", Size: 9},
		Alignment: &excelize.Alignment{WrapText: true, Vertical: "top"},
	})
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// ──────────────── Sheet 1: 角色属性 ────────────────

func writeSheetCharAttrs(f *excelize.File, sty *styles, chars []charJSON) error {
	const sheet = "角色属性"
	f.SetSheetName("Sheet1", sheet)

	headers := []string{
		"角色名称", "角色ID", "最大生命", "最大能量",
		"解放阈值", "解放方式", "被动·攻击加伤", "被动·受击减伤", "被动·濒死拦截",
	}
	descs := []string{
		"显示名称", "英文标识(勿改)", "整数", "整数",
		"0=无解放", "手动/自动/无", "整数,0=无", "整数,0=无", "是/否",
	}
	if err := writeHeaderAndDesc(f, sheet, headers, descs, sty); err != nil {
		return err
	}

	for r, c := range chars {
		row := r + 3
		// 解放方式反译
		libMode := "自动"
		switch {
		case c.LibThreshold == 0:
			libMode = "无"
		case c.ManualLib:
			libMode = "手动"
		}
		vals := []any{
			c.Name, c.ID, c.MaxHP, c.MaxEnergy,
			c.LibThreshold, libMode,
			intOrBlank(c.Passive.BonusOutgoing),
			intOrBlank(c.Passive.IncomingReduction),
			boolOrBlank(c.Passive.InterceptNearDeath),
		}
		if err := writeRow(f, sheet, row, 1, vals); err != nil {
			return err
		}
	}

	if err := f.SetColWidth(sheet, "A", "A", 14); err != nil {
		return err
	}
	if err := f.SetColWidth(sheet, "B", "B", 16); err != nil {
		return err
	}
	if err := f.SetColWidth(sheet, "C", "F", 10); err != nil {
		return err
	}
	if err := f.SetColWidth(sheet, "G", "I", 14); err != nil {
		return err
	}
	return f.SetPanes(sheet, &excelize.Panes{
		Freeze: true, XSplit: 0, YSplit: 2, TopLeftCell: "A3", ActivePane: "bottomLeft",
	})
}

// ──────────────── Sheet 2: 角色技能 ────────────────

func writeSheetCharSkills(f *excelize.File, sty *styles, chars []charJSON) error {
	const sheet = "角色技能"
	if _, err := f.NewSheet(sheet); err != nil {
		return err
	}

	headers := []string{
		"角色名称", "技能等级", "技能名称", "能量消耗",
		"造成伤害", "回复生命", "获得能量", "摸牌数", "自伤", "技能描述",
	}
	descs := []string{
		"与【角色属性】一致", "普通/强化/解放", "技能显示名", "整数",
		"整数,0=无", "整数,0=无", "整数,0=无", "整数,0=无", "整数,0=无", "技能说明文字",
	}
	if err := writeHeaderAndDesc(f, sheet, headers, descs, sty); err != nil {
		return err
	}

	row := 3
	tiers := []struct {
		level string
		s     func(c charJSON) skillJSON
	}{
		{"普通", func(c charJSON) skillJSON { return c.Normal }},
		{"强化", func(c charJSON) skillJSON { return c.Enhanced }},
		{"解放", func(c charJSON) skillJSON { return c.Lib }},
	}

	// 先找出最后一个有技能的角色，用于决定何时不再加分隔行。
	lastWithSkills := -1
	for i, c := range chars {
		for _, t := range tiers {
			if !skillIsEmpty(t.s(c)) {
				lastWithSkills = i
				break
			}
		}
	}

	for ci, c := range chars {
		hasAny := false
		for _, t := range tiers {
			if !skillIsEmpty(t.s(c)) {
				hasAny = true
				break
			}
		}
		if !hasAny {
			continue
		}
		for _, t := range tiers {
			s := t.s(c)
			if skillIsEmpty(s) {
				continue
			}
			vals := []any{
				c.Name, t.level, s.Name, intOrBlank(s.EnergyCost),
				intOrBlank(s.Result.DealDirectDamage),
				intOrBlank(s.Result.HealSelf),
				intOrBlank(s.Result.GainEnergy),
				intOrBlank(s.Result.DrawCards),
				intOrBlank(s.Result.DamageSelf),
				s.Result.Desc,
			}
			if err := writeRow(f, sheet, row, 1, vals); err != nil {
				return err
			}
			row++
		}
		if ci < lastWithSkills {
			if err := paintSeparator(f, sheet, row, 10, sty.sep); err != nil {
				return err
			}
			row++
		}
	}

	if err := f.SetColWidth(sheet, "A", "A", 14); err != nil {
		return err
	}
	if err := f.SetColWidth(sheet, "B", "B", 10); err != nil {
		return err
	}
	if err := f.SetColWidth(sheet, "C", "C", 14); err != nil {
		return err
	}
	if err := f.SetColWidth(sheet, "D", "I", 10); err != nil {
		return err
	}
	if err := f.SetColWidth(sheet, "J", "J", 50); err != nil {
		return err
	}
	return f.SetPanes(sheet, &excelize.Panes{
		Freeze: true, XSplit: 0, YSplit: 2, TopLeftCell: "A3", ActivePane: "bottomLeft",
	})
}

// ──────────────── Sheet 3: 特殊机制 ────────────────

func writeSheetHooks(f *excelize.File, sty *styles, chars []charJSON) error {
	const sheet = "特殊机制"
	if _, err := f.NewSheet(sheet); err != nil {
		return err
	}

	headers := []string{"角色名称", "参数名称", "参数值", "说明（仅供参考，无需填写）"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		if err := f.SetCellValue(sheet, cell, h); err != nil {
			return err
		}
		if err := f.SetCellStyle(sheet, cell, cell, sty.header); err != nil {
			return err
		}
	}

	// 找出最后一个含 hooks_config 的角色，用于决定是否需要分隔行。
	lastWithHooks := -1
	for i, c := range chars {
		if len(c.HooksConfig) > 0 {
			lastWithHooks = i
		}
	}

	row := 2
	for ci, c := range chars {
		if len(c.HooksConfig) == 0 {
			continue
		}
		type kv struct {
			cn       string
			val      any
			dataType string
			ord      int
		}
		const tail = 1 << 30
		items := make([]kv, 0, len(c.HooksConfig))
		for jsonKey, val := range c.HooksConfig {
			cn, dt, ok := excelmap.ReverseLookup(c.Name, jsonKey)
			if !ok {
				log.Printf("⚠ 角色[%s] 的 hooks_config key [%s] 未在 excelmap 注册，"+
					"已用英文 key 占位输出；请到 internal/excelmap/params.go 补上中英文映射。", c.Name, jsonKey)
				items = append(items, kv{cn: jsonKey, val: val, dataType: detectType(val), ord: tail})
				continue
			}
			ord, ok := excelmap.ParamOrderIndex[cn]
			if !ok {
				ord = tail
				log.Printf("⚠ 参数[%s] 未在 HooksParamOrder 中排序，将放到角色[%s]的末尾。", cn, c.Name)
			}
			items = append(items, kv{cn: cn, val: val, dataType: dt, ord: ord})
		}
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].ord != items[j].ord {
				return items[i].ord < items[j].ord
			}
			return items[i].cn < items[j].cn
		})

		for _, it := range items {
			cellVal, err := formatParamValue(it.val, it.dataType)
			if err != nil {
				return fmt.Errorf("角色[%s] 参数[%s]: %w", c.Name, it.cn, err)
			}
			vals := []any{c.Name, it.cn, cellVal, excelmap.HooksParamNotes[it.cn]}
			if err := writeRow(f, sheet, row, 1, vals); err != nil {
				return err
			}
			noteCell, _ := excelize.CoordinatesToCellName(4, row)
			if err := f.SetCellStyle(sheet, noteCell, noteCell, sty.note); err != nil {
				return err
			}
			row++
		}
		if ci < lastWithHooks {
			if err := paintSeparator(f, sheet, row, 4, sty.sep); err != nil {
				return err
			}
			row++
		}
	}

	if err := f.SetColWidth(sheet, "A", "A", 14); err != nil {
		return err
	}
	if err := f.SetColWidth(sheet, "B", "B", 20); err != nil {
		return err
	}
	if err := f.SetColWidth(sheet, "C", "C", 16); err != nil {
		return err
	}
	if err := f.SetColWidth(sheet, "D", "D", 40); err != nil {
		return err
	}
	return f.SetPanes(sheet, &excelize.Panes{
		Freeze: true, XSplit: 0, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft",
	})
}

// ──────────────── Sheet 4: 场地效果 ────────────────

func writeSheetFields(f *excelize.File, sty *styles, fields []fieldJSON) error {
	const sheet = "场地效果"
	if _, err := f.NewSheet(sheet); err != nil {
		return err
	}

	headers := []string{
		"场地名称", "场地ID", "虚幻加成", "允许同系合成",
		"轮回规则", "隐藏摸牌", "攻击加成", "濒死吸取",
	}
	descs := []string{
		"显示名称", "英文标识(勿改)", "是/否", "是/否",
		"0=无/1=基础/2=变体", "是/否", "整数,0=无", "整数,0=无",
	}
	if err := writeHeaderAndDesc(f, sheet, headers, descs, sty); err != nil {
		return err
	}

	for r, fld := range fields {
		row := r + 3
		vals := []any{
			fld.Name, fld.ID,
			boolOrBlank(fld.IllusionBonus),
			boolOrBlank(fld.AllowSameType),
			fld.ReincarnRule,
			boolOrBlank(fld.HideDrawnCards),
			intOrBlank(fld.BonusAttack),
			intOrBlank(fld.NearDeathDrain),
		}
		if err := writeRow(f, sheet, row, 1, vals); err != nil {
			return err
		}
	}

	if err := f.SetColWidth(sheet, "A", "A", 16); err != nil {
		return err
	}
	if err := f.SetColWidth(sheet, "B", "B", 16); err != nil {
		return err
	}
	if err := f.SetColWidth(sheet, "C", "H", 14); err != nil {
		return err
	}
	return f.SetPanes(sheet, &excelize.Panes{
		Freeze: true, XSplit: 0, YSplit: 2, TopLeftCell: "A3", ActivePane: "bottomLeft",
	})
}

// ────────────────────────────────────────────────────────────────
//  通用辅助
// ────────────────────────────────────────────────────────────────

func writeHeaderAndDesc(f *excelize.File, sheet string, headers, descs []string, sty *styles) error {
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		if err := f.SetCellValue(sheet, cell, h); err != nil {
			return err
		}
		if err := f.SetCellStyle(sheet, cell, cell, sty.header); err != nil {
			return err
		}
	}
	for i, d := range descs {
		cell, _ := excelize.CoordinatesToCellName(i+1, 2)
		if err := f.SetCellValue(sheet, cell, d); err != nil {
			return err
		}
		if err := f.SetCellStyle(sheet, cell, cell, sty.desc); err != nil {
			return err
		}
	}
	return nil
}

// writeRow 从 (row, startCol) 起按 vals 顺序写入。空字符串单元格不创建。
func writeRow(f *excelize.File, sheet string, row, startCol int, vals []any) error {
	for i, v := range vals {
		cell, _ := excelize.CoordinatesToCellName(startCol+i, row)
		if s, ok := v.(string); ok && s == "" {
			continue
		}
		if err := f.SetCellValue(sheet, cell, v); err != nil {
			return err
		}
	}
	return nil
}

func paintSeparator(f *excelize.File, sheet string, row, colCount, styleID int) error {
	for c := 1; c <= colCount; c++ {
		cell, _ := excelize.CoordinatesToCellName(c, row)
		if err := f.SetCellStyle(sheet, cell, cell, styleID); err != nil {
			return err
		}
	}
	return nil
}

// intOrBlank 0 → 空字符串（避免 Excel 中显示一片 0），其它值原样。
func intOrBlank(n int) any {
	if n == 0 {
		return ""
	}
	return n
}

// boolOrBlank true → "是"，false → 空字符串。
func boolOrBlank(b bool) any {
	if b {
		return "是"
	}
	return ""
}

// skillIsEmpty 用于跳过那些未定义任何技能的"hook-only"角色。
func skillIsEmpty(s skillJSON) bool {
	return s.Name == "" && s.EnergyCost == 0 &&
		s.Result.DealDirectDamage == 0 && s.Result.HealSelf == 0 &&
		s.Result.GainEnergy == 0 && s.Result.DrawCards == 0 &&
		s.Result.DamageSelf == 0 && s.Result.Desc == ""
}

// detectType 反向 fallback：当某个 jsonKey 没在映射表里时，根据 JSON 实际类型猜测。
func detectType(v any) string {
	switch v.(type) {
	case bool:
		return "bool"
	case []any:
		return "int_list"
	default:
		return "int"
	}
}

// formatParamValue 把任意 JSON 解析后的值按 dataType 转成 Excel 单元格字符串。
//   - int       → 整数字符串
//   - bool      → "是" / "否"
//   - int_list  → "1,2,3"
func formatParamValue(v any, dataType string) (string, error) {
	switch dataType {
	case "int":
		n, ok := toInt(v)
		if !ok {
			return "", fmt.Errorf("期望 int，实际 %T(%v)", v, v)
		}
		return strconv.Itoa(n), nil
	case "bool":
		b, ok := v.(bool)
		if !ok {
			return "", fmt.Errorf("期望 bool，实际 %T(%v)", v, v)
		}
		if b {
			return "是", nil
		}
		return "否", nil
	case "int_list":
		arr, ok := v.([]any)
		if !ok {
			return "", fmt.Errorf("期望 []any，实际 %T(%v)", v, v)
		}
		parts := make([]string, 0, len(arr))
		for _, item := range arr {
			n, ok := toInt(item)
			if !ok {
				return "", fmt.Errorf("int_list 元素非整数：%T(%v)", item, item)
			}
			parts = append(parts, strconv.Itoa(n))
		}
		return strings.Join(parts, ","), nil
	default:
		return "", fmt.Errorf("未知数据类型 %q", dataType)
	}
}

// toInt 把 JSON 解析出的数值（默认 float64）安全转成 int。
func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	}
	return 0, false
}
