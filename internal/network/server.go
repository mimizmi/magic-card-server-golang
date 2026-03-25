package network

import (
	"log/slog"
	"net"
	"sync"
)

// Server 负责监听 TCP 端口，为每个连接创建 Session 并管理其生命周期。
type Server struct {
	addr   string
	router *Router

	// sessions 存储所有活跃会话。
	// 使用 sync.Map 而不是 map+Mutex，原因：
	//   - 写操作（注册/注销）远少于读操作（广播时遍历）
	//   - sync.Map 对这种读多写少的场景有更好的性能
	//   - 避免在 Accept 循环和 removeSession 之间出现死锁
	sessions sync.Map // map[string]*Session
}

// NewServer 创建服务器，addr 格式如 "0.0.0.0:8080"。
func NewServer(addr string, router *Router) *Server {
	return &Server{
		addr:   addr,
		router: router,
	}
}

// Start 开始监听并阻塞接受连接，通常在主 goroutine 调用。
// 出错时返回 error（例如端口被占用）。
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	slog.Info("server started", "addr", s.addr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			// Accept 出错通常是 listener 被关闭，记录后退出
			slog.Error("accept error", "err", err)
			return err
		}

		sess := newSession(conn, s.router, s)
		s.sessions.Store(sess.ID, sess)
		slog.Info("new connection", "sessionID", sess.ID, "remote", conn.RemoteAddr())

		// 每个连接独立一个 goroutine，互不阻塞
		// 这是 Go 服务器的标准模式：goroutine-per-connection
		go sess.run()
	}
}

// removeSession 在会话断开后由 Session.run() 的 defer 调用，清理映射表。
func (s *Server) removeSession(sess *Session) {
	s.sessions.Delete(sess.ID)
}

// Broadcast 向所有在线会话广播一条消息。
// 游戏中用于：场地效果公告、结算结果等需要所有人知道的事件。
func (s *Server) Broadcast(msgID uint16, payload []byte) {
	s.sessions.Range(func(_, v any) bool {
		v.(*Session).Send(msgID, payload)
		return true // 返回 true 继续遍历
	})
}

// GetSession 根据 ID 查找会话，找不到返回 nil。
func (s *Server) GetSession(id string) *Session {
	v, ok := s.sessions.Load(id)
	if !ok {
		return nil
	}
	return v.(*Session)
}

// SessionCount 返回当前在线连接数，可用于监控。
func (s *Server) SessionCount() int {
	count := 0
	s.sessions.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}
