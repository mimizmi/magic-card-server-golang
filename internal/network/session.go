package network

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// sendChanSize 发送队列缓冲大小。
	// 缓冲区满时说明客户端消费太慢，此时应断开而非无限堆积。
	sendChanSize = 64

	// pingInterval 服务器主动向客户端发送 Ping 的间隔。
	pingInterval = 15 * time.Second

	// pongTimeout 超过此时间未收到 Pong，判定连接已死。
	// 必须 > pingInterval，给客户端足够的响应时间。
	pongTimeout = 35 * time.Second
)

// 心跳消息 ID 约定（客户端需遵守相同约定）
const (
	MsgIDPing uint16 = 1 // 服务端 -> 客户端
	MsgIDPong uint16 = 2 // 客户端 -> 服务端
)

// 全局会话 ID 计数器，使用原子操作保证并发安全且不重复
var sessionIDCounter atomic.Uint64

// Session 代表一个客户端的长连接会话。
//
// 并发模型：
//   - readLoop  goroutine：负责从 conn 读取数据，驱动业务逻辑
//   - writeLoop goroutine：独占 conn 的写权限，从 sendCh 取数据写出
//   - heartbeatLoop goroutine：定时发 Ping，检查 Pong 超时
//
// 为什么写操作需要单独的 goroutine？
//   net.Conn.Write 不是并发安全的。如果多个 goroutine 同时写同一个连接，
//   数据会交叉损坏。通过 sendCh channel，我们把所有写操作序列化到一个
//   goroutine，其他任何地方只需往 channel 塞数据，不直接碰 conn。
type Session struct {
	// ID 是会话唯一标识，创建后不可变，可在多 goroutine 中安全读取
	ID string

	conn   net.Conn
	router *Router
	server *Server

	// sendCh 是写队列。所有想发送数据的地方都往这里塞，writeLoop 消费。
	sendCh chan []byte

	// ctx/cancel 用于优雅关闭：cancel() 一调，所有监听 ctx.Done() 的
	// goroutine 都会退出。
	ctx    context.Context
	cancel context.CancelFunc

	// lastPongAt 记录最后一次收到 Pong 的时间，需要加锁访问。
	lastPongAt time.Time
	mu         sync.Mutex

	// closeOnce 保证关闭逻辑只执行一次（避免重复 close channel）
	closeOnce sync.Once

	// OnClose 是会话关闭时的可选回调，由上层模块（PlayerManager）注入。
	// 用途：玩家下线时，让 PlayerManager 清理状态、通知匹配队列等。
	// 注意：在 closeOnce 内部调用，保证只执行一次，且在 conn.Close() 之后。
	OnClose func()
}

func newSession(conn net.Conn, router *Router, srv *Server) *Session {
	ctx, cancel := context.WithCancel(context.Background())
	id := fmt.Sprintf("sess-%d", sessionIDCounter.Add(1))
	return &Session{
		ID:         id,
		conn:       conn,
		router:     router,
		server:     srv,
		sendCh:     make(chan []byte, sendChanSize),
		ctx:        ctx,
		cancel:     cancel,
		lastPongAt: time.Now(),
	}
}

// Send 将一条消息放入发送队列，非阻塞。
// 若队列已满（客户端积压），直接断开连接而不是阻塞服务端。
func (s *Session) Send(msgID uint16, payload []byte) {
	frame := EncodeFrame(msgID, payload)
	select {
	case s.sendCh <- frame:
	default:
		slog.Warn("send buffer full, closing session", "sessionID", s.ID)
		s.Close()
	}
}

// Close 关闭会话，幂等，多次调用安全。
func (s *Session) Close() {
	s.closeOnce.Do(func() {
		s.cancel()     // 通知所有 goroutine 退出
		s.conn.Close() // 解除 readLoop 的 io.ReadFull 阻塞
		if s.OnClose != nil {
			s.OnClose()
		}
	})
}

// run 启动会话的所有 goroutine，在 readLoop 返回前阻塞。
// 由 Server 在 Accept 后以 go sess.run() 的方式调用。
func (s *Session) run() {
	defer func() {
		s.Close()
		s.server.removeSession(s)
		slog.Info("session disconnected", "sessionID", s.ID, "remote", s.conn.RemoteAddr())
	}()

	go s.writeLoop()
	go s.heartbeatLoop()
	s.readLoop() // 阻塞直到连接断开或出错
}

// readLoop 持续从连接读取帧，分发给 router 处理。
// 只有这一个地方读 conn，不存在并发读。
func (s *Session) readLoop() {
	for {
		frame, err := ReadFrame(s.conn)
		if err != nil {
			// 连接断开是正常情况（客户端主动关闭），不打 Error 级别日志
			slog.Debug("read frame error", "sessionID", s.ID, "err", err)
			return
		}

		// Pong 是心跳响应，直接在这里处理，不走 router
		if frame.MsgID == MsgIDPong {
			s.mu.Lock()
			s.lastPongAt = time.Now()
			s.mu.Unlock()
			continue
		}

		s.router.dispatch(s, frame.MsgID, frame.Payload)
	}
}

// writeLoop 是唯一写 conn 的地方。
// 通过监听 sendCh 和 ctx.Done() 两个 channel 来工作。
func (s *Session) writeLoop() {
	for {
		select {
		case data := <-s.sendCh:
			if _, err := s.conn.Write(data); err != nil {
				slog.Debug("write error", "sessionID", s.ID, "err", err)
				s.Close()
				return
			}
		case <-s.ctx.Done():
			return
		}
	}
}

// heartbeatLoop 定期发 Ping，检查 Pong 是否超时。
func (s *Session) heartbeatLoop() {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.mu.Lock()
			timeout := time.Since(s.lastPongAt) > pongTimeout
			s.mu.Unlock()

			if timeout {
				slog.Info("heartbeat timeout", "sessionID", s.ID)
				s.Close()
				return
			}
			s.Send(MsgIDPing, nil)

		case <-s.ctx.Done():
			return
		}
	}
}
