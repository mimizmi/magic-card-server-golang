package card

import (
	"fmt"
	"sync/atomic"
)

// ════════════════════════════════════════════════════════════════
//  大系（Major Faction）
// ════════════════════════════════════════════════════════════════

// MajorFaction 是牌的大类归属，决定合成时用乘算还是加算。
type MajorFaction int8

const (
	MajorFantasy MajorFaction = 0 // 梦幻系（梦境 + 虚幻）
	MajorReturn  MajorFaction = 1 // 重回系（重组 + 轮回）
)

func (m MajorFaction) String() string {
	switch m {
	case MajorFantasy:
		return "梦幻"
	case MajorReturn:
		return "重回"
	default:
		return "未知"
	}
}

// ════════════════════════════════════════════════════════════════
//  子系（SubFaction）
// ════════════════════════════════════════════════════════════════

// SubFaction 是牌的具体派系，两个大系各含两个子系。
// 子系信息在场地效果中有用（轮回之境、虚幻之境分别针对轮回牌、虚幻牌）。
type SubFaction int8

const (
	SubDream         SubFaction = 0 // 梦境（梦幻系）
	SubIllusion      SubFaction = 1 // 虚幻（梦幻系）
	SubReform        SubFaction = 2 // 重组（重回系）
	SubReincarnation SubFaction = 3 // 轮回（重回系）
)

// Major 返回该子系所属的大系。
func (sf SubFaction) Major() MajorFaction {
	if sf == SubDream || sf == SubIllusion {
		return MajorFantasy
	}
	return MajorReturn
}

func (sf SubFaction) String() string {
	switch sf {
	case SubDream:
		return "梦境"
	case SubIllusion:
		return "虚幻"
	case SubReform:
		return "重组"
	case SubReincarnation:
		return "轮回"
	default:
		return "未知"
	}
}

// ════════════════════════════════════════════════════════════════
//  功能牌型（CardType）
// ════════════════════════════════════════════════════════════════

// CardType 是牌的功能属性，决定这张牌能做什么。
// 注意：功能牌型与大系是正交的，一张牌同时具有两种属性。
// 例：梦境·攻击牌（子系=梦境，功能=攻击）
type CardType int8

const (
	TypeAttack CardType = 0 // 攻击牌：交战阶段用，点数即伤害值
	TypeSkill  CardType = 1 // 技能牌：持有并达到门槛时触发角色技能，使用后消耗
	TypeEnergy CardType = 2 // 能耗牌（解放）：点数转化为等量能量值
)

func (ct CardType) String() string {
	switch ct {
	case TypeAttack:
		return "攻击"
	case TypeSkill:
		return "技能"
	case TypeEnergy:
		return "能耗"
	default:
		return "未知"
	}
}

// ════════════════════════════════════════════════════════════════
//  点数常量
// ════════════════════════════════════════════════════════════════

const (
	MinPoints = 1 // 牌的最低点数
	MaxPoints = 5 // 标准上限（合成结果超过此值时截断）

	// MaxPointsWithField 是虚幻之境·实场地效果下的突破上限。
	// 仅适用于该场地效果激活期间的合成操作。
	MaxPointsWithField = 7
)

// ════════════════════════════════════════════════════════════════
//  Card — 单张牌
// ════════════════════════════════════════════════════════════════

// Card 代表游戏中的一张具体的牌。
//
// 设计为值类型（struct，非 pointer），原因：
//   - 牌在各区域间移动时（手牌→合成区）本质上是"搬运"，值拷贝语义更直觉
//   - 合成操作产生新牌，不修改原有两张牌，值语义避免意外的共享状态
//   - HandZone 内部用 *Card（指针）存储，是为了区分"有牌"（非nil）和"空槽"（nil）
//
// IsHidden 与点数的关系：
//   - IsHidden = true 时，该牌的点数对对手不可见（虚幻之境·虚场地效果）
//   - 牌主自己永远能看到自己的牌
//   - 协议层（CardView.Points = nil）用于表达这个状态
type Card struct {
	ID         string
	SubFaction SubFaction
	CardType   CardType
	Points     int  // 1-5（合成后可能最高 7）
	IsHidden   bool // 点数对对手隐藏（场地效果触发）
}

func (c *Card) String() string {
	hidden := ""
	if c.IsHidden {
		hidden = "[隐藏]"
	}
	return fmt.Sprintf("[%s·%s %d点%s]", c.SubFaction, c.CardType, c.Points, hidden)
}

// ════════════════════════════════════════════════════════════════
//  Card ID 生成
// ════════════════════════════════════════════════════════════════

var cardIDCounter atomic.Uint64

// newCardID 生成全局唯一的牌 ID。
// 格式：card-{递增序号}，轻量且无需随机数。
func newCardID() string {
	return fmt.Sprintf("card-%d", cardIDCounter.Add(1))
}

// New 创建一张新牌（主要由 Deck 调用）。
func New(sf SubFaction, ct CardType, points int) *Card {
	return &Card{
		ID:         newCardID(),
		SubFaction: sf,
		CardType:   ct,
		Points:     clamp(points, MinPoints, MaxPoints),
	}
}

func clamp(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
