package tun

import (
	"github.com/0990/gotun/pkg/stats"
	"net"
	"sync/atomic"
)

// 统计流量的包装器
type StatsConn struct {
	net.Conn

	readBytes  uint64
	writeBytes uint64

	readCounter  stats.Counter
	writeCounter stats.Counter
}

func (c *StatsConn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	if n > 0 {
		atomic.AddUint64(&c.readBytes, uint64(n))
		c.readCounter.Add(int64(n))
	}
	return n, err
}

func (c *StatsConn) Write(b []byte) (int, error) {
	n, err := c.Conn.Write(b)
	if n > 0 {
		atomic.AddUint64(&c.writeBytes, uint64(n))
		c.writeCounter.Add(int64(n))
	}
	return n, err
}

// 获取总读写字节数
func (c *StatsConn) BytesRead() uint64 {
	return atomic.LoadUint64(&c.readBytes)
}

func (c *StatsConn) BytesWritten() uint64 {
	return atomic.LoadUint64(&c.writeBytes)
}
