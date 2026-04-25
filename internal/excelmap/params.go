// Package excelmap 提供策划 Excel 字段（中文）与服务端 JSON 字段（英文）之间的双向映射。
//
// 两个工具共用本包，避免映射表脱节：
//   - cmd/exceltojson —— 把策划填好的 Excel 转成 JSON（构建期跑，参考 Dockerfile）
//   - cmd/jsontoexcel —— 把开发者新加的 JSON 反向同步回 Excel（开发完顺手跑）
//
// 添加新角色的特殊机制参数时：
//  1. 在 HooksParamMap 增加 中文名 → {jsonKey, dataType} 条目；
//  2. 若该 jsonKey 与已有条目冲突，再用 HooksParamOverrides 给该角色单独指定。
package excelmap

// ParamDef 描述一个特殊机制参数。
type ParamDef struct {
	JSONKey  string // characters.json 里 hooks_config 下的英文 key
	DataType string // "int" | "bool" | "int_list"
}

// HooksParamMap 中文参数名 → 英文 JSON key + 类型。
// 注：同一个中文名对应同一个 JSON key（除非在 HooksParamOverrides 中按角色覆盖）。
var HooksParamMap = map[string]ParamDef{
	// 时空裂缝者
	"HP与能量共享":  {JSONKey: "hp_energy_shared", DataType: "bool"},
	"初始生命值":    {JSONKey: "init_hp", DataType: "int"},
	"初始能量值":    {JSONKey: "init_energy", DataType: "int"},
	"裂缝基础能量":   {JSONKey: "default_rift_bonus", DataType: "int"},
	"裂缝能量递增":   {JSONKey: "rift_bonus_increment", DataType: "int"},
	"普通技能消耗":   {JSONKey: "normal_skill_cost", DataType: "int"},
	"强化技能消耗":   {JSONKey: "enhanced_skill_cost", DataType: "int"},
	"强化技能点数阈值": {JSONKey: "enhanced_skill_pts_threshold", DataType: "int"},
	"超能解放能量阈值": {JSONKey: "liberation_energy_threshold", DataType: "int"},
	// 万能者
	"全牌攻击化":   {JSONKey: "all_cards_as_attack", DataType: "bool"},
	"阶段伤害阈值":  {JSONKey: "phase_thresholds", DataType: "int_list"},
	"阶段1攻击加成": {JSONKey: "phase1_attack_bonus", DataType: "int"},
	"阶段2牌面加成": {JSONKey: "phase2_card_bonus", DataType: "int"},
	"阶段3攻击倍率": {JSONKey: "phase3_attack_multiplier", DataType: "int"},
	// 血魔
	"受伤牌面加成阈值": {JSONKey: "dmg_received_threshold", DataType: "int"},
	"受伤后牌面加成":  {JSONKey: "dmg_received_card_bonus", DataType: "int"},
	"吸血伤害阈值":   {JSONKey: "lifesteal_damage_threshold", DataType: "int"},
	"吸血激活点数":   {JSONKey: "lifesteal_activate_pts", DataType: "int"},
	"强化技能攻击加成": {JSONKey: "enhanced_atk_bonus", DataType: "int"},
	"强化技能自伤":   {JSONKey: "enhanced_self_damage", DataType: "int"},
	"普通技能自伤":   {JSONKey: "normal_self_damage", DataType: "int"},
	"普通技能摸牌数":  {JSONKey: "normal_draw_cards", DataType: "int"},
	// 反伤者
	"反弹层数上限":   {JSONKey: "max_reflect_stacks", DataType: "int"},
	"强化免疫阶段数":  {JSONKey: "enhanced_immune_phases", DataType: "int"},
	"解放技能消耗":   {JSONKey: "lib_cost", DataType: "int"},
	"解放技能点数阈值": {JSONKey: "lib_pts_threshold", DataType: "int"},
	"解放免疫阶段数":  {JSONKey: "lib_immune_phases", DataType: "int"},
	// 建造者
	"工人基础效率":   {JSONKey: "base_worker_eff", DataType: "int"},
	"一层房效率加成":  {JSONKey: "house1_eff_bonus", DataType: "int"},
	"二层房减伤":    {JSONKey: "house2_dmg_reduction", DataType: "int"},
	"三层房回血":    {JSONKey: "house3_heal", DataType: "int"},
	"房子减半伤害阈值": {JSONKey: "damage_halve_threshold", DataType: "int"},
	"解放最低房子数":  {JSONKey: "lib_house_threshold", DataType: "int"},
	"解放最低点数":   {JSONKey: "lib_pts_min", DataType: "int"},
	"解放最高点数":   {JSONKey: "lib_pts_max", DataType: "int"},
	// 积怨者
	"解锁伤害阈值": {JSONKey: "unlock_threshold", DataType: "int"},
	"解锁后补抽数": {JSONKey: "unlocked_draw", DataType: "int"},
}

// HooksParamOverrides 当同一个中文名在不同角色映射到不同 JSON key 时使用。
// 例：「强化技能点数阈值」在时空裂缝者里是 enhanced_skill_pts_threshold，
//     在血魔/反伤者里是 enhanced_pts_threshold（历史遗留命名）。
var HooksParamOverrides = map[string]map[string]string{
	"反伤者": {
		"普通技能消耗":   "normal_cost",
		"强化技能消耗":   "enhanced_cost",
		"强化技能点数阈值": "enhanced_pts_threshold",
	},
	"血魔": {
		"强化技能点数阈值": "enhanced_pts_threshold",
	},
}

// HooksParamNotes 中文参数名 → 给策划看的「说明」文本（写入"特殊机制"sheet 第 D 列）。
// 只在 jsontoexcel 反向生成时使用；缺项时该参数的说明列留空。
var HooksParamNotes = map[string]string{
	// 时空裂缝者
	"HP与能量共享":  "是=HP和能量共用一个数值池",
	"初始生命值":    "开局实际HP（而非最大值）",
	"初始能量值":    "开局实际能量",
	"裂缝基础能量":   "每个裂缝提供的基础能量加成",
	"裂缝能量递增":   "每多一个裂缝，加成额外增加的量",
	"普通技能消耗":   "开启裂缝消耗的能量",
	"强化技能消耗":   "强化裂缝消耗的能量",
	"强化技能点数阈值": "需要多少合成点才能使用强化技能",
	"超能解放能量阈值": "能量达到此值时触发超能解放",
	// 万能者
	"全牌攻击化":   "是=所有卡牌都视为攻击牌",
	"阶段伤害阈值":  "逗号分隔，依次为阶段1/2/3的伤害阈值",
	"阶段1攻击加成": "阶段1时每次攻击额外伤害",
	"阶段2牌面加成": "阶段2时出牌点数加成",
	"阶段3攻击倍率": "阶段3时攻击伤害倍率",
	// 血魔
	"受伤牌面加成阈值": "累计受伤达到此值后，牌面获得加成",
	"受伤后牌面加成":  "触发后每张牌额外加成点数",
	"吸血伤害阈值":   "累计造成伤害达到此值后可激活吸血",
	"吸血激活点数":   "激活吸血需要的合成点数",
	"强化技能攻击加成": "使用强化技能时额外攻击伤害",
	"强化技能自伤":   "使用强化技能时自伤的HP",
	"普通技能自伤":   "使用普通技能时自伤的HP",
	"普通技能摸牌数":  "使用普通技能时摸牌张数",
	// 反伤者
	"反弹层数上限":  "反伤护盾可叠加的最大层数",
	"强化免疫阶段数": "技能免疫持续的阶段数",
	"解放技能消耗":  "解放技能消耗的能量",
	"解放技能点数阈值": "触发解放需要的最低技能牌点数",
	"解放免疫阶段数": "全免疫+反弹持续的阶段数",
	// 建造者
	"工人基础效率":   "每个小人每阶段建造的进度（基础值）",
	"一层房效率加成":  "超过1层房子时每层房子增加的工人效率",
	"二层房减伤":    "超过2层房子时每层房子增加的减伤值",
	"三层房回血":    "达到3层房子时每层房子每回合回血量",
	"房子减半伤害阈值": "单次伤害超过此值时房子减半",
	"解放最低房子数":  "触发解放需要的最低房子层数",
	"解放最低点数":   "触发解放需要的最低技能牌点数",
	"解放最高点数":   "触发解放的最高技能牌点数",
	// 积怨者
	"解锁伤害阈值": "任一原始牌型累计伤害达到此值，该牌型攻击转为不可防御",
	"解锁后补抽数": "解锁牌型成功命中后补抽的牌数",
}

// CharOrder 是 Excel 各 sheet 默认的角色排列顺序（与 gen_template.go 保持一致）。
// 反向生成时按此顺序输出；JSON 中存在但本表未列入的角色会按 JSON 顺序追加在末尾。
var CharOrder = []string{
	"力裁者", "镜换者", "空手者", "噬渊者", "灼血者", "殉道者",
	"时空裂缝者", "万能者", "血魔", "反伤者", "建造者", "积怨者",
}

// HooksParamOrder 是反向生成"特殊机制"sheet 时，每个角色内部参数行的稳定排序权重。
// 不在此列表中的参数，会按字母序追加在该角色的最后，并打印 warning，提示开发者补充。
var HooksParamOrder = []string{
	// 时空裂缝者
	"HP与能量共享", "初始生命值", "初始能量值", "裂缝基础能量", "裂缝能量递增",
	"普通技能消耗", "强化技能消耗", "强化技能点数阈值", "超能解放能量阈值",
	// 万能者
	"全牌攻击化", "阶段伤害阈值", "阶段1攻击加成", "阶段2牌面加成", "阶段3攻击倍率",
	// 血魔
	"受伤牌面加成阈值", "受伤后牌面加成", "吸血伤害阈值", "吸血激活点数",
	"强化技能攻击加成", "强化技能自伤", "普通技能自伤", "普通技能摸牌数",
	// 反伤者
	"反弹层数上限", "解放技能消耗", "解放技能点数阈值", "强化免疫阶段数", "解放免疫阶段数",
	// 建造者
	"工人基础效率", "一层房效率加成", "二层房减伤", "三层房回血", "房子减半伤害阈值",
	"解放最低房子数", "解放最低点数", "解放最高点数",
	// 积怨者
	"解锁伤害阈值", "解锁后补抽数",
}

// ParamOrderIndex 把 HooksParamOrder 转成 中文名 → 序号 的查表，便于排序时 O(1) 查询。
var ParamOrderIndex = func() map[string]int {
	m := make(map[string]int, len(HooksParamOrder))
	for i, name := range HooksParamOrder {
		m[name] = i
	}
	return m
}()

// ResolveJSONKey 给定中文参数名 + 角色中文名，返回该角色实际写入 JSON 的英文 key。
// 第二返回值表明该中文名是否注册在 HooksParamMap 中。
func ResolveJSONKey(charName, paramName string) (string, ParamDef, bool) {
	def, ok := HooksParamMap[paramName]
	if !ok {
		return "", ParamDef{}, false
	}
	jsonKey := def.JSONKey
	if overrides, ok := HooksParamOverrides[charName]; ok {
		if override, ok := overrides[paramName]; ok {
			jsonKey = override
		}
	}
	return jsonKey, def, true
}

// ReverseLookup 给定角色中文名 + JSON key，返回 (中文名, 类型, 是否找到)。
// 反向工具（jsontoexcel）用它把 JSON 数据反译为策划友好的中文字段。
//
// 实现策略：
//  1. 优先查 HooksParamOverrides[charName] —— 如果该角色有特殊映射，覆盖通用映射；
//  2. 否则在 HooksParamMap 中搜第一个 JSONKey 匹配的中文名。
//
// 返回的 ok=false 表示这个 JSON key 既不在覆盖表也不在主映射，调用方应回退到
// "原样输出英文 key + 标注未注册"，避免静默丢失数据。
func ReverseLookup(charName, jsonKey string) (string, string, bool) {
	if overrides, ok := HooksParamOverrides[charName]; ok {
		for cn, jk := range overrides {
			if jk == jsonKey {
				if def, ok := HooksParamMap[cn]; ok {
					return cn, def.DataType, true
				}
				return cn, "int", true
			}
		}
	}
	// 该角色被某条 override 改写过的中文名要排除掉，避免误把通用名归还给它
	for cn, def := range HooksParamMap {
		if def.JSONKey != jsonKey {
			continue
		}
		if overrides, ok := HooksParamOverrides[charName]; ok {
			if _, redirected := overrides[cn]; redirected {
				continue
			}
		}
		return cn, def.DataType, true
	}
	return "", "", false
}
