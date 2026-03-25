package protocol

// 消息 ID 分段规划：
//
//   1  -  99  : 系统级（心跳）
//   1000-1099  : 认证（登录）
//   2000-2099  : 匹配系统
//   3000-3099  : 游戏状态同步（服务端 → 客户端）
//   4000-4099  : 玩家操作（客户端 → 服务端）
//   5000-5099  : 游戏事件推送（服务端 → 客户端，单向通知）
//
// 命名规范：
//   MsgXxxReq  = 客户端请求
//   MsgXxxResp = 服务端响应（对应 Req 的直接答复）
//   MsgXxxEv   = 服务端主动推送的事件（不对应某个 Req）

const (
	// ── 系统 ──────────────────────────────────────────────
	MsgPing uint16 = 1 // S→C 心跳探测
	MsgPong uint16 = 2 // C→S 心跳响应

	// ── 认证 ──────────────────────────────────────────────
	MsgLoginReq  uint16 = 1001 // C→S 登录
	MsgLoginResp uint16 = 1002 // S→C 登录结果

	// ── 匹配 ──────────────────────────────────────────────
	MsgJoinQueueReq       uint16 = 2001 // C→S 加入匹配队列
	MsgJoinQueueResp      uint16 = 2002 // S→C 入队结果
	MsgLeaveQueueReq      uint16 = 2003 // C→S 取消匹配
	MsgMatchFoundEv       uint16 = 2004 // S→C 匹配成功，进入角色选择
	MsgSelectCharacterReq uint16 = 2005 // C→S 选择角色（暗置）
	MsgGameStartEv        uint16 = 2006 // S→C 双方均已选角，游戏正式开始

	// ── 游戏状态同步 ───────────────────────────────────────
	MsgGameStateEv  uint16 = 3001 // S→C 完整状态快照（阶段开始时发送）
	MsgPhaseChangeEv uint16 = 3002 // S→C 阶段切换通知

	// ── 玩家操作（行动阶段内有效） ────────────────────────────
	MsgPlayCardReq        uint16 = 4001 // C→S 使用攻击牌/能耗牌（从手牌区或合成区）
	MsgMoveToSynthReq     uint16 = 4002 // C→S 将手牌移入合成区
	MsgSynthesizeReq      uint16 = 4003 // C→S 合成两张牌
	MsgUseSkillReq        uint16 = 4004 // C→S 使用主动技能
	MsgTriggerLibrateReq  uint16 = 4005 // C→S 手动触发解放（适用于主动解放型角色）
	MsgEndActionReq       uint16 = 4006 // C→S 宣告结束行动阶段
	MsgDefenseReq         uint16 = 4007 // C→S 防御出牌（响应来袭攻击，Pass=true 表示不防御）

	// ── 游戏事件推送 ───────────────────────────────────────
	MsgDamageEv       uint16 = 5001 // S→C 伤害结算明细
	MsgSkillUsedEv    uint16 = 5002 // S→C 技能使用（同时公开角色身份）
	MsgLiberationEv   uint16 = 5003 // S→C 解放触发
	MsgFieldEffectEv  uint16 = 5004 // S→C 场地效果生效
	MsgPlayerStatusEv uint16 = 5005 // S→C HP/能量变化（增量更新，不发完整状态）
	MsgGameOverEv     uint16 = 5006 // S→C 游戏结束
	MsgErrorEv        uint16 = 5007 // S→C 操作错误反馈（非法操作、时序错误等）
	MsgBlessingEv          uint16 = 5008 // S→C 赐福触发（HP<40，获得第二角色）
	MsgIncomingAttackEv    uint16 = 5009 // S→C 来袭攻击通知（等待防御）
)
