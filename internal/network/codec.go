package network

import (
	"encoding/binary"
	"io"
)

// HeaderSize 是每帧固定头部长度：4字节消息体长度 + 2字节消息ID
const HeaderSize = 6

// Frame 代表一条完整的网络消息。
// 协议格式：[ 4字节 body长度(uint32 BE) ][ 2字节 msgID(uint16 BE) ][ N字节 payload ]
type Frame struct {
	MsgID   uint16
	Payload []byte
}

// ReadFrame 从 r 中读取一个完整帧。
// 使用 io.ReadFull 保证在网络分包的情况下也能读到完整数据。
// 返回 error 时调用方应关闭连接，不要重试。
func ReadFrame(r io.Reader) (*Frame, error) {
	header := make([]byte, HeaderSize)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}

	bodyLen := binary.BigEndian.Uint32(header[0:4])
	msgID := binary.BigEndian.Uint16(header[4:6])

	// 防御：单帧上限 1MB，避免客户端构造超大帧耗尽内存
	const maxBodyLen = 1 << 20
	if bodyLen > maxBodyLen {
		return nil, io.ErrUnexpectedEOF
	}

	payload := make([]byte, bodyLen)
	if bodyLen > 0 {
		if _, err := io.ReadFull(r, payload); err != nil {
			return nil, err
		}
	}

	return &Frame{MsgID: msgID, Payload: payload}, nil
}

// EncodeFrame 将 msgID 和 payload 打包为一个完整帧字节切片，可直接写入 conn。
// 合并头部和 payload 为一次 Write，减少系统调用次数。
func EncodeFrame(msgID uint16, payload []byte) []byte {
	buf := make([]byte, HeaderSize+len(payload))
	binary.BigEndian.PutUint32(buf[0:4], uint32(len(payload)))
	binary.BigEndian.PutUint16(buf[4:6], msgID)
	copy(buf[HeaderSize:], payload)
	return buf
}
