package network

import "log/slog"

// HandlerFunc 是所有消息处理函数的统一签名。
// s    - 发送该消息的会话，处理函数通过 s.Send() 回复
// data - 消息体原始字节，上层模块负责反序列化
type HandlerFunc func(s *Session, data []byte)

// Router 将消息 ID 映射到对应的处理函数。
// 读写在服务器启动前完成注册，运行时只读，因此不需要加锁。
type Router struct {
	handlers map[uint16]HandlerFunc
}

// NewRouter 创建一个空路由器。
func NewRouter() *Router {
	return &Router{handlers: make(map[uint16]HandlerFunc)}
}

// Register 注册一个消息 ID 对应的处理函数。
// 应在服务器 Start() 之前完成所有注册。
func (r *Router) Register(msgID uint16, h HandlerFunc) {
	r.handlers[msgID] = h
}

// dispatch 根据消息 ID 找到处理函数并调用。
// 找不到对应 handler 时记录警告日志并丢弃，不断开连接。
// 注意：dispatch 在 session 的 readLoop goroutine 中同步执行，
// 如果 handler 有耗时操作应自行起新 goroutine。
func (r *Router) dispatch(s *Session, msgID uint16, data []byte) {
	h, ok := r.handlers[msgID]
	if !ok {
		slog.Warn("unknown msgID", "msgID", msgID, "sessionID", s.ID)
		return
	}
	h(s, data)
}
