package tun

import (
	"github.com/0990/gotun/core"
	"github.com/0990/gotun/pkg/stats"
	"net"
	"sync"
)

var udpBufPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, core.MaxSegmentSize)
	},
}

type UDPConn struct {
	net.Conn
	buf    []byte
	offset int
	limit  int
}

func (c *UDPConn) ID() string {
	return "udpconn"
}

func (c *UDPConn) Read(b []byte) (int, error) {
	// 如果 buffer 已经读完，就重新填充
	if c.offset >= c.limit {
		if c.buf != nil {
			// 用完归还池子
			udpBufPool.Put(c.buf)
		}
		c.buf = udpBufPool.Get().([]byte)
		n, err := c.Conn.Read(c.buf)
		if err != nil {
			udpBufPool.Put(c.buf) // 失败时也归还
			c.buf = nil
			return n, err
		}
		c.offset = 0
		c.limit = n
	}

	// 从 buf 复制数据到 b
	n := copy(b, c.buf[c.offset:c.limit])
	c.offset += n
	return n, nil
}

func dialUDP(addr string, config string, readerCounter, writeCounter stats.Counter) (core.IStream, error) {
	raddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}

	udpConn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return nil, err
	}

	conn := &StatsConn{Conn: udpConn, readCounter: readerCounter, writeCounter: writeCounter}

	return &UDPConn{Conn: conn}, nil
}
