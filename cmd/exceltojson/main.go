// cmd/exceltojson 是一个小工具，将策划填写的 Excel 配置表导出为服务端/客户端使用的 JSON 数据文件。
//
// Excel 工作表结构（全中文，策划友好）：
//   - 角色属性：基础数值（角色名称、角色ID、最大生命……）
//   - 角色技能：每个技能一行（角色名称、技能等级、技能名称……）
//   - 特殊机制：hooks参数（角色名称、参数名称、参数值）
//   - 场地效果：场地配置（场地名称、场地ID……）
//
// 用法：
//
//	go run ./cmd/exceltojson -i data/游戏配置表.xlsx -o data/
//
// 会在 -o 目录下生成：
//   - characters.json （角色配置）
//   - fields.json     （场地效果配置）
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"echo/internal/excelmap"

	"github.com/xuri/excelize/v2"
)

func main() {
	input := flag.String("i", "data/游戏配置表.xlsx", "输入 Excel 文件路径")
	output := flag.String("o", "data/", "输出目录")
	flag.Parse()

	f, err := excelize.OpenFile(*input)
	if err != nil {
		log.Fatalf("打开 Excel 失败: %v", err)
	}
	defer f.Close()

	// 1) 解析角色属性
	charMap, charOrder, err := parseCharAttrs(f)
	if err != nil {
		log.Fatalf("解析[角色属性]失败: %v", err)
	}

	// 2) 解析角色技能，挂到对应角色上
	if err := parseCharSkills(f, charMap); err != nil {
		log.Fatalf("解析[角色技能]失败: %v", err)
	}

	// 3) 解析特殊机制，挂到对应角色上
	if err := parseHooksConfig(f, charMap); err != nil {
		log.Fatalf("解析[特殊机制]失败: %v", err)
	}

	// 按原始顺序输出
	chars := make([]map[string]any, 0, len(charOrder))
	for _, name := range charOrder {
		chars = append(chars, charMap[name])
	}

	if err := writeJSON(filepath.Join(*output, "characters.json"), chars); err != nil {
		log.Fatalf("写入 characters.json 失败: %v", err)
	}
	fmt.Printf("✓ 已导出 %d 个角色 → %s\n", len(chars), filepath.Join(*output, "characters.json"))

	// 4) 解析场地效果
	fields, err := parseFields(f)
	if err != nil {
		log.Fatalf("解析[场地效果]失败: %v", err)
	}
	if err := writeJSON(filepath.Join(*output, "fields.json"), fields); err != nil {
		log.Fatalf("写入 fields.json 失败: %v", err)
	}
	fmt.Printf("✓ 已导出 %d 个场地效果 → %s\n", len(fields), filepath.Join(*output, "fields.json"))
}

// ════════════════════════════════════════════════════════════════
//  Sheet 1: 角色属性
// ════════════════════════════════════════════════════════════════
//
// 列: A:角色名称 B:角色ID C:最大生命 D:最大能量 E:解放阈值
//     F:解放方式 G:被动·攻击加伤 H:被动·受击减伤 I:被动·濒死拦截
// 第1行=表头，第2行=填写说明，数据从第3行开始

func parseCharAttrs(f *excelize.File) (map[string]map[string]any, []string, error) {
	const sheet = "角色属性"
	rows, err := f.GetRows(sheet)
	if err != nil {
		return nil, nil, fmt.Errorf("读取工作表[%s]: %w", sheet, err)
	}
	if len(rows) < 3 {
		return nil, nil, fmt.Errorf("工作表[%s]至少需要表头+说明行+1行数据", sheet)
	}

	charMap := make(map[string]map[string]any)
	var order []string

	for _, row := range rows[2:] { // 跳过表头+说明行
		name := getCell(row, 0)
		if name == "" {
			continue
		}
		id := getCell(row, 1)
		if id == "" {
			continue
		}

		libMode := getCell(row, 5)
		manualLib := false
		libThreshold := getCellInt(row, 4)
		switch libMode {
		case "手动":
			manualLib = true
		case "自动":
			manualLib = false
		case "无", "":
			manualLib = false
			libThreshold = 0
		}

		ch := map[string]any{
			"id":            id,
			"name":          name,
			"max_hp":        getCellInt(row, 2),
			"max_energy":    getCellInt(row, 3),
			"lib_threshold": libThreshold,
			"manual_lib":    manualLib,
		}

		// 被动
		passive := map[string]any{}
		if v := getCellInt(row, 6); v != 0 {
			passive["bonus_outgoing"] = v
		}
		if v := getCellInt(row, 7); v != 0 {
			passive["incoming_reduction"] = v
		}
		if getCellBool(row, 8) {
			passive["intercept_near_death"] = true
		}
		ch["passive"] = passive

		// 初始化空技能（后续由 Sheet2 填充）
		ch["normal"] = map[string]any{}
		ch["enhanced"] = map[string]any{}
		ch["lib"] = map[string]any{}

		charMap[name] = ch
		order = append(order, name)
	}

	return charMap, order, nil
}

// ════════════════════════════════════════════════════════════════
//  Sheet 2: 角色技能
// ════════════════════════════════════════════════════════════════
//
// 列: A:角色名称 B:技能等级(普通/强化/解放) C:技能名称 D:能量消耗
//     E:造成伤害 F:回复生命 G:获得能量 H:摸牌数 I:自伤 J:技能描述
// 第1行=表头，第2行=填写说明，数据从第3行开始

func parseCharSkills(f *excelize.File, charMap map[string]map[string]any) error {
	const sheet = "角色技能"
	rows, err := f.GetRows(sheet)
	if err != nil {
		return fmt.Errorf("读取工作表[%s]: %w", sheet, err)
	}

	// 技能等级 → JSON key 映射
	levelMap := map[string]string{
		"普通": "normal",
		"强化": "enhanced",
		"解放": "lib",
	}

	for i, row := range rows[2:] { // 跳过表头+说明行
		name := getCell(row, 0)
		if name == "" {
			continue // 空行或分隔行
		}
		level := getCell(row, 1)
		jsonKey, ok := levelMap[level]
		if !ok {
			continue // 无效等级，跳过
		}

		ch, exists := charMap[name]
		if !exists {
			return fmt.Errorf("第%d行: 角色[%s]在[角色属性]表中不存在", i+3, name)
		}

		skill := buildSkill(
			getCell(row, 2),    // 技能名称
			getCellInt(row, 3), // 能量消耗
			getCellInt(row, 4), // 造成伤害
			getCellInt(row, 5), // 回复生命
			getCellInt(row, 6), // 获得能量
			getCellInt(row, 7), // 摸牌数
			getCellInt(row, 8), // 自伤
			getCell(row, 9),    // 技能描述
		)

		ch[jsonKey] = skill
	}

	return nil
}

// ════════════════════════════════════════════════════════════════
//  Sheet 3: 特殊机制
// ════════════════════════════════════════════════════════════════
//
// 列: A:角色名称 B:参数名称 C:参数值 D:说明
// 第1行=表头，数据从第2行开始
//
// 中文参数名 → 英文 JSON key 的映射关系由 echo/internal/excelmap 维护，
// 与反向工具 cmd/jsontoexcel 共用同一份事实，避免脱节。

func parseHooksConfig(f *excelize.File, charMap map[string]map[string]any) error {
	const sheet = "特殊机制"
	rows, err := f.GetRows(sheet)
	if err != nil {
		return fmt.Errorf("读取工作表[%s]: %w", sheet, err)
	}

	// 收集每个角色的 hooks_config
	hooksConfigs := make(map[string]map[string]any)

	for i, row := range rows[1:] { // 跳过表头
		charName := getCell(row, 0)
		paramName := getCell(row, 1)
		paramValue := getCell(row, 2)

		if charName == "" || paramName == "" {
			continue // 空行或分隔行
		}

		jsonKey, def, ok := excelmap.ResolveJSONKey(charName, paramName)
		if !ok {
			return fmt.Errorf("第%d行: 未知参数名[%s]", i+2, paramName)
		}

		if _, exists := charMap[charName]; !exists {
			return fmt.Errorf("第%d行: 角色[%s]在[角色属性]表中不存在", i+2, charName)
		}

		if hooksConfigs[charName] == nil {
			hooksConfigs[charName] = make(map[string]any)
		}

		// 按类型解析值
		switch def.DataType {
		case "int":
			n, err := strconv.Atoi(strings.TrimSpace(paramValue))
			if err != nil {
				return fmt.Errorf("第%d行: 参数[%s]应为整数，实际值[%s]", i+2, paramName, paramValue)
			}
			hooksConfigs[charName][jsonKey] = n
		case "bool":
			hooksConfigs[charName][jsonKey] = paramValue == "是" || paramValue == "true" || paramValue == "TRUE" || paramValue == "1"
		case "int_list":
			parts := strings.Split(paramValue, ",")
			var nums []int
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p == "" {
					continue
				}
				n, err := strconv.Atoi(p)
				if err != nil {
					return fmt.Errorf("第%d行: 参数[%s]应为逗号分隔的整数列表，实际值[%s]", i+2, paramName, paramValue)
				}
				nums = append(nums, n)
			}
			hooksConfigs[charName][jsonKey] = nums
		}
	}

	// 挂载到角色数据上
	for charName, hc := range hooksConfigs {
		charMap[charName]["hooks_config"] = hc
	}

	return nil
}

// ════════════════════════════════════════════════════════════════
//  Sheet 4: 场地效果
// ════════════════════════════════════════════════════════════════
//
// 列: A:场地名称 B:场地ID C:虚幻加成 D:允许同系合成
//     E:轮回规则 F:隐藏摸牌 G:攻击加成 H:濒死吸取
// 第1行=表头，第2行=填写说明，数据从第3行开始

func parseFields(f *excelize.File) ([]map[string]any, error) {
	const sheet = "场地效果"
	rows, err := f.GetRows(sheet)
	if err != nil {
		return nil, fmt.Errorf("读取工作表[%s]: %w", sheet, err)
	}
	if len(rows) < 3 {
		return nil, fmt.Errorf("工作表[%s]至少需要表头+说明行+1行数据", sheet)
	}

	var result []map[string]any
	for _, row := range rows[2:] { // 跳过表头+说明行
		name := getCell(row, 0)
		if name == "" {
			continue
		}
		id := getCell(row, 1)
		if id == "" {
			continue
		}

		fe := map[string]any{
			"id":   id,
			"name": name,
		}
		if getCellBool(row, 2) {
			fe["illusion_bonus"] = true
		}
		if getCellBool(row, 3) {
			fe["allow_same_type"] = true
		}
		if v := getCellInt(row, 4); v != 0 {
			fe["reincarn_rule"] = v
		}
		if getCellBool(row, 5) {
			fe["hide_drawn_cards"] = true
		}
		if v := getCellInt(row, 6); v != 0 {
			fe["bonus_attack"] = v
		}
		if v := getCellInt(row, 7); v != 0 {
			fe["near_death_drain"] = v
		}

		result = append(result, fe)
	}
	return result, nil
}

// ════════════════════════════════════════════════════════════════
//  工具函数
// ════════════════════════════════════════════════════════════════

func getCell(row []string, col int) string {
	if col < len(row) {
		return strings.TrimSpace(row[col])
	}
	return ""
}

func getCellInt(row []string, col int) int {
	s := getCell(row, col)
	if s == "" {
		return 0
	}
	n, _ := strconv.Atoi(s)
	return n
}

func getCellBool(row []string, col int) bool {
	s := getCell(row, col)
	return s == "是" || s == "true" || s == "TRUE" || s == "1"
}

func buildSkill(name string, cost, damage, heal, energy, draw, selfDmg int, desc string) map[string]any {
	skill := map[string]any{}
	if name != "" {
		skill["name"] = name
	}
	if cost != 0 {
		skill["energy_cost"] = cost
	}
	result := map[string]any{}
	if damage != 0 {
		result["deal_direct_damage"] = damage
	}
	if heal != 0 {
		result["heal_self"] = heal
	}
	if energy != 0 {
		result["gain_energy"] = energy
	}
	if draw != 0 {
		result["draw_cards"] = draw
	}
	if selfDmg != 0 {
		result["damage_self"] = selfDmg
	}
	if desc != "" {
		result["desc"] = desc
	}
	if len(result) > 0 {
		skill["result"] = result
	}
	return skill
}

func writeJSON(path string, data any) error {
	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0644)
}
