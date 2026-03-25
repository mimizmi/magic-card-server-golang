package player

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"echo/internal/network"
)

// ════════════════════════════════════════════════════════════════
//  Player — 玩家实体
// ════════════════════════════════════════════════════════════════

// Player 代表一个游戏玩家，生命周期长于单次 TCP 连接。
//
// 关键设计：Player 和 Session 是分开的。
//   - Session 是"一根网线"，断了就没了
//   - Player 是"这个玩家"，断线后仍然存在一段时间（等待重连）
//
// 这个分离让断线重连成为可能：
//   旧 Session 断开 → Player 标记 offline → 新 Session 连入 → Player 重绑 Session
type Player struct {
	ID   string
	Name string

	mu      sync.Mutex
	session *network.Session // 当前活跃 Session，nil 表示离线
	roomID  string           // 所在房间 ID，"" 表示未在游戏中
}

// Send 向玩家发送消息。
// 若玩家当前离线（session == nil），消息静默丢弃。
// Phase 4 可以在这里加"离线消息队列"，重连后补发。
func (p *Player) Send(msgID uint16, payload []byte) {
	p.mu.Lock()
	sess := p.session
	p.mu.Unlock()
	if sess != nil {
		sess.Send(msgID, payload)
	}
}

func (p *Player) setSession(sess *network.Session) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.session = sess
}

// IsOnline 返回玩家是否有活跃连接。
func (p *Player) IsOnline() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.session != nil
}

// SetRoom 绑定玩家到房间，"" 表示离开房间。
func (p *Player) SetRoom(roomID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.roomID = roomID
}

// RoomID 返回玩家当前所在房间 ID。
func (p *Player) RoomID() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.roomID
}

// ════════════════════════════════════════════════════════════════
//  reconnectEntry — 断线重连凭证
// ════════════════════════════════════════════════════════════════

type reconnectEntry struct {
	player    *Player
	expiresAt time.Time
}

// reconnectTTL 是断线后保留重连凭证的时间。
// 超过此时间玩家无法重连，对局判负。
const reconnectTTL = 3 * time.Minute

// ════════════════════════════════════════════════════════════════
//  Manager — 玩家管理器
// ════════════════════════════════════════════════════════════════

// Manager 负责玩家的注册、Session 绑定与断线重连。
//
// 内部维护三张表：
//   players    playerID  → *Player  （全量，包含离线玩家）
//   bySession  sessionID → *Player  （当前在线的 Session → Player 映射）
//   byToken    token     → reconnectEntry  （断线重连凭证）
type Manager struct {
	players   sync.Map // playerID  → *Player
	bySession sync.Map // sessionID → *Player
	byToken   sync.Map // token     → reconnectEntry

	// disconnectHooks 是断线回调链，匹配队列等模块在此注册清理逻辑。
	// 读多写少（启动时注册，运行时只读），用 RWMutex。
	hookMu          sync.RWMutex
	disconnectHooks []func(*Player)
}

// NewManager 创建玩家管理器。
func NewManager() *Manager {
	return &Manager{}
}

// OnDisconnect 注册一个在任意玩家断线时调用的回调。
// 应在服务器启动前注册（注册后不再变化）。
func (m *Manager) OnDisconnect(fn func(*Player)) {
	m.hookMu.Lock()
	defer m.hookMu.Unlock()
	m.disconnectHooks = append(m.disconnectHooks, fn)
}

// Register 为新连接的玩家创建 Player 实体并绑定 Session。
// 返回新玩家和其断线重连 token（客户端需持久化保存）。
func (m *Manager) Register(name string, sess *network.Session) (p *Player, token string) {
	p = &Player{
		ID:      fmt.Sprintf("p-%s", generateToken()[:8]),
		Name:    name,
		session: sess,
	}
	token = generateToken()

	m.players.Store(p.ID, p)
	m.bySession.Store(sess.ID, p)
	m.byToken.Store(token, reconnectEntry{player: p, expiresAt: time.Now().Add(reconnectTTL)})

	// 注入断线回调：Session 关闭时，Player 标记离线并通知上层
	sess.OnClose = func() {
		m.handleDisconnect(sess.ID, p)
	}

	slog.Info("player registered", "playerID", p.ID, "name", name, "sessionID", sess.ID)
	return p, token
}

// Reconnect 凭 token 将已有 Player 重绑到新 Session。
// token 无效或已过期返回 nil。
func (m *Manager) Reconnect(token string, sess *network.Session) *Player {
	v, ok := m.byToken.Load(token)
	if !ok {
		return nil
	}
	entry := v.(reconnectEntry)
	if time.Now().After(entry.expiresAt) {
		m.byToken.Delete(token)
		return nil
	}

	p := entry.player
	p.setSession(sess)
	m.bySession.Store(sess.ID, p)

	// 延长 token 有效期（重连成功后继续保留，以防再次掉线）
	m.byToken.Store(token, reconnectEntry{player: p, expiresAt: time.Now().Add(reconnectTTL)})

	sess.OnClose = func() {
		m.handleDisconnect(sess.ID, p)
	}

	slog.Info("player reconnected", "playerID", p.ID, "name", p.Name, "sessionID", sess.ID)
	return p
}

// GetBySession 根据 Session ID 查找对应的 Player。
func (m *Manager) GetBySession(sessionID string) *Player {
	v, ok := m.bySession.Load(sessionID)
	if !ok {
		return nil
	}
	return v.(*Player)
}

// handleDisconnect 在 Session 关闭时由 OnClose 回调调用。
func (m *Manager) handleDisconnect(sessionID string, p *Player) {
	m.bySession.Delete(sessionID)
	p.setSession(nil)
	slog.Info("player offline", "playerID", p.ID, "name", p.Name)

	// 通知所有注册了 OnDisconnect 的上层模块
	m.hookMu.RLock()
	hooks := m.disconnectHooks
	m.hookMu.RUnlock()
	for _, fn := range hooks {
		fn(p)
	}
}

// generateToken 生成 32 字符的随机十六进制字符串。
func generateToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
