package protocol

import (
	"encoding/json"
	"fmt"
)

// ════════════════════════════════════════════════════════════════
//  编解码工具
// ════════════════════════════════════════════════════════════════

// Encode 将任意消息序列化为 JSON 字节，作为帧的 payload。
func Encode(msg any) ([]byte, error) {
	return json.Marshal(msg)
}

// MustEncode 序列化消息，失败则 panic。
// 只对服务端自己构造的消息使用（结构固定，理论上不会失败）。
// 不要用于处理客户端传入的数据。
func MustEncode(msg any) []byte {
	b, err := json.Marshal(msg)
	if err != nil {
		panic("protocol.MustEncode: " + err.Error())
	}
	return b
}

// Decode 将 JSON 字节反序列化为 T 类型。
// 使用 Go 1.18+ 泛型，调用方无需类型断言。
//
// 示例：
//
//	req, err := protocol.Decode[LoginReq](data)
//	if err != nil { ... }
//	// req 的类型已经是 *LoginReq
func Decode[T any](data []byte) (*T, error) {
	var msg T
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("decode %T: %w", msg, err)
	}
	return &msg, nil
}

// ════════════════════════════════════════════════════════════════
//  认证消息
// ════════════════════════════════════════════════════════════════

// LoginReq C→S 登录请求。
// 首次登录：填写 PlayerName，留空 ReconnectToken。
// 断线重连：填写之前收到的 ReconnectToken（PlayerName 可选）。
type LoginReq struct {
	PlayerName     string `json:"player_name"`
	ReconnectToken string `json:"reconnect_token,omitempty"`
}

// LoginResp S→C 登录结果。
type LoginResp struct {
	Success        bool   `json:"success"`
	PlayerID       string `json:"player_id,omitempty"`        // 玩家唯一 ID
	ReconnectToken string `json:"reconnect_token,omitempty"`  // 客户端必须持久化，断线后凭此重连
	InGame         bool   `json:"in_game,omitempty"`          // true 表示重连后仍在对局中
	Error          string `json:"error,omitempty"`
}

// ════════════════════════════════════════════════════════════════
//  匹配消息
// ════════════════════════════════════════════════════════════════

// JoinQueueReq C→S 请求加入匹配队列。
type JoinQueueReq struct {
	PlayerID string `json:"player_id"`
}

// JoinQueueResp S→C 入队结果。
type JoinQueueResp struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// MatchFoundEv S→C 匹配成功事件。
// 收到此消息后客户端进入选角界面（Phase 3 补充选角消息）。
type MatchFoundEv struct {
	GameID       string `json:"game_id"`
	YourSeat     int    `json:"your_seat"`     // 0 或 1，决定先后手
	OpponentName string `json:"opponent_name"`
}

// SelectCharacterReq C→S 选择角色（暗置）。
// 收到 MatchFoundEv 后客户端发送此消息，服务端等双方都选好后开始游戏。
// CharacterID 为角色标识符，合法值见角色系统定义。
type SelectCharacterReq struct {
	CharacterID string `json:"character_id"`
}

// GameStartEv S→C 游戏正式开始通知。
// 包含双方座位和初始状态，客户端收到后切换到游戏界面。
type GameStartEv struct {
	GameID string `json:"game_id"`
	// Seat0Char / Seat1Char 均为 "???"（角色暗置），使用技能后才公开
	Seat0Char string `json:"seat0_char"`
	Seat1Char string `json:"seat1_char"`
}

// ════════════════════════════════════════════════════════════════
//  玩家操作请求（行动阶段内有效）
// ════════════════════════════════════════════════════════════════

// PlayCardReq C→S 使用一张牌（攻击牌打出参与交战，能耗牌转化能量）。
// Zone: "hand"（手牌区）或 "synth"（合成区）
// Slot: 1-8（hand zone）或 1-4（synth zone）
type PlayCardReq struct {
	Zone string `json:"zone"`
	Slot int    `json:"slot"`
}

// MoveToSynthReq C→S 将手牌区的牌移入合成区（为合成做准备）。
// 只有手牌区的牌可以移入合成区，合成区最多容纳 4 张。
type MoveToSynthReq struct {
	HandSlot int `json:"hand_slot"` // 1-8
}

// SynthesizeReq C→S 合成两张牌。
// 合成规则：同大系→点数相乘；不同大系→点数相加；同种牌型→禁止合成。
// 至少有一张牌必须在合成区（手牌区的牌要先 MoveToSynth）。
type SynthesizeReq struct {
	Slot1 int    `json:"slot1"`
	Zone1 string `json:"zone1"` // "hand" or "synth"
	Slot2 int    `json:"slot2"`
	Zone2 string `json:"zone2"`
}

// UseSkillReq C→S 使用主动技能。
// SkillCardSlot: 手牌区中技能牌的槽位（1-8），其点数决定触发一级还是二级效果。
type UseSkillReq struct {
	SkillCardSlot int `json:"skill_card_slot"`
}

// TriggerLibrateReq C→S 手动触发解放。
// 仅适用于"主动触发"型解放的角色（力裁者、镜换者、空手者、噬渊者、灼血者）。
// 殉道者的解放为自动触发，不需要此消息。
type TriggerLibrateReq struct{}

// EndActionReq C→S 宣告结束自己的行动阶段。
// 双方都结束行动后，进入交战结算阶段。
type EndActionReq struct{}

// DefenseReq C→S 防御出牌，响应对手的来袭攻击。
// Pass=true：放弃防御，承受全部伤害。
// Pass=false：用指定槽位的牌抵消对应点数的伤害。
type DefenseReq struct {
	Pass bool   `json:"pass"`
	Zone string `json:"zone,omitempty"` // "hand" or "synth"
	Slot int    `json:"slot,omitempty"`
}

// IncomingAttackEv S→C 来袭攻击通知（防御窗口开启时推送）。
type IncomingAttackEv struct {
	AttackerSeat int `json:"attacker_seat"`
	AttackPoints int `json:"attack_points"`
}

// ════════════════════════════════════════════════════════════════
//  游戏事件（服务端主动推送）
// ════════════════════════════════════════════════════════════════

// DamageEv S→C 伤害结算明细。
// 推送给双方，让双方都能看到伤害过程（含反弹、吸收等修正）。
type DamageEv struct {
	AttackerSeat int    `json:"attacker_seat"`
	DefenderSeat int    `json:"defender_seat"`
	RawDamage    int    `json:"raw_damage"`   // 原始伤害（攻击牌点数）
	FinalDamage  int    `json:"final_damage"` // 实际扣血（经反弹/吸收/减免后）
	HPAfter      int    `json:"hp_after"`     // 受击方结算后的 HP
	Detail       string `json:"detail"`       // 说明，如"攻击牌伤害"/"殉道反弹"/"噬渊吸收"
}

// SkillUsedEv S→C 技能使用事件。
// 使用技能时角色身份公开，此消息同时承担"公开角色"的作用。
type SkillUsedEv struct {
	PlayerSeat int    `json:"player_seat"`
	Character  string `json:"character"`   // 角色名称
	SkillLevel int    `json:"skill_level"` // 1 or 2
	Desc       string `json:"desc"`        // 效果描述（面向客户端展示）
}

// LiberationEv S→C 解放技能触发事件。
type LiberationEv struct {
	PlayerSeat int    `json:"player_seat"`
	Character  string `json:"character"`
	Desc       string `json:"desc"`
}

// FieldEffectEv S→C 场地效果生效事件（每阶段开始时）。
type FieldEffectEv struct {
	EffectID   string `json:"effect_id"`   // 内部标识，如 "illusory_real"
	EffectName string `json:"effect_name"` // 展示名，如 "虚幻之境·实"
	Desc       string `json:"desc"`
}

// PlayerStatusEv S→C HP 或能量变化的增量通知。
// 用于不需要发完整状态快照的小更新，减少数据量。
type PlayerStatusEv struct {
	Seat      int `json:"seat"`
	HP        int `json:"hp"`
	MaxHP     int `json:"max_hp"`
	Energy    int `json:"energy"`
	MaxEnergy int `json:"max_energy"`
}

// PhaseChangeEv S→C 阶段切换通知。
type PhaseChangeEv struct {
	Round       int    `json:"round"`
	Phase       string `json:"phase"`        // "field_draw","draw","action","combat","cleanup"
	ActiveSeat  int    `json:"active_seat"`  // 当前该谁操作（行动阶段轮流）
	FieldEffect string `json:"field_effect"` // 本回合场地效果名称（空串 = 无效果）
}

// BlessingEv S→C 赐福触发（HP < 40 时获得第二角色）。
type BlessingEv struct {
	PlayerSeat    int    `json:"player_seat"`
	SecondCharID  string `json:"second_char_id"`   // 第二角色 ID
	SecondCharName string `json:"second_char_name"` // 第二角色显示名
}

// GameOverEv S→C 游戏结束。
type GameOverEv struct {
	WinnerSeat int    `json:"winner_seat"`
	Reason     string `json:"reason"` // "hp_zero" / "secret_realm_highest_hp"
}

// TurnTimerEv S→C 行动倒计时通知，每秒推送一次，行动阶段结束后停止。
type TurnTimerEv struct {
	ActiveSeat  int `json:"active_seat"`  // 当前行动方
	SecondsLeft int `json:"seconds_left"` // 剩余秒数（0-60）
}

// ErrorEv S→C 操作错误反馈（非致命，连接不断开）。
// 例如：在非行动阶段出牌、合成区已满、不满足技能门槛等。
type ErrorEv struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// 错误码定义
const (
	ErrCodeInvalidPhase    = 1001 // 当前阶段不允许此操作
	ErrCodeInvalidSlot     = 1002 // 无效槽位
	ErrCodeNoCard          = 1003 // 指定槽位没有牌
	ErrCodeSynthSameType   = 1004 // 同种牌型无法合成
	ErrCodeSynthAlready    = 1009 // 合成产物不可再次合成
	ErrCodeSkillNoCard     = 1005 // 手中无技能牌
	ErrCodeSkillThreshold  = 1006 // 技能牌点数不足
	ErrCodeLibrateNotReady = 1007 // 能量未达到解放阈值
	ErrCodeNotYourTurn     = 1008 // 不是你的行动回合
)
