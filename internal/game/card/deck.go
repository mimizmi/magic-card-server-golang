package card

import (
	"math/rand"
	"time"
)

// allSubFactions 和 allCardTypes 是抽牌时随机选取的候选集合。
// 使用切片而非直接用 Intn(4) 以便将来调整概率权重（比如让攻击牌更多见）。
var allSubFactions = []SubFaction{
	SubDream,
	SubIllusion,
	SubReform,
	SubReincarnation,
}

var allCardTypes = []CardType{
	TypeAttack,
	TypeSkill,
	TypeEnergy,
}

// Deck 是一个无限随机牌堆。
//
// 游戏规则中没有"构筑牌组"的设定，牌堆按需随机生成。
// 每次 Draw 产生一张随机牌，来自 4 个子系 × 3 种功能 × 5 个点数的组合空间。
//
// 为什么不用 crypto/rand？
//   游戏随机性不需要密码学安全，math/rand 速度更快。
//   每个 Deck 有独立的 rng，保证两局游戏的随机序列互不干扰。
//
// 为什么 Deck 不是全局单例？
//   每个 GameState 持有自己的 Deck，并记录初始 seed。
//   这让"录像回放"成为可能：用相同 seed 重建 Deck，重放所有操作，
//   得到完全一致的牌序列。（Phase 6 后续扩展）
type Deck struct {
	rng  *rand.Rand
	seed int64
}

// NewDeck 用随机 seed 创建牌堆，适合正常对局。
func NewDeck() *Deck {
	seed := time.Now().UnixNano()
	return NewDeckWithSeed(seed)
}

// NewDeckWithSeed 用指定 seed 创建牌堆，用于测试或录像回放。
func NewDeckWithSeed(seed int64) *Deck {
	return &Deck{
		rng:  rand.New(rand.NewSource(seed)), //nolint:gosec
		seed: seed,
	}
}

// Seed 返回创建时使用的随机种子，可用于录像存档。
func (d *Deck) Seed() int64 {
	return d.seed
}

// Draw 从牌堆取一张随机牌。
// 子系、功能牌型、点数均匀随机，无权重偏置。
func (d *Deck) Draw() *Card {
	sf := allSubFactions[d.rng.Intn(len(allSubFactions))]
	ct := allCardTypes[d.rng.Intn(len(allCardTypes))]
	pts := d.rng.Intn(MaxPoints) + MinPoints // [1, 5]
	return New(sf, ct, pts)
}

// DrawN 连续抽取 n 张牌，顺序即为抽取顺序。
func (d *Deck) DrawN(n int) []*Card {
	cards := make([]*Card, n)
	for i := range cards {
		cards[i] = d.Draw()
	}
	return cards
}
