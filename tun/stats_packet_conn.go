package tun

import (
	"github.com/0990/gotun/pkg/stats"
	"net"
)

type StatsPacketConn struct {
	net.PacketConn
	readCounter  stats.Counter
	writeCounter stats.Counter
}

func (c *StatsPacketConn) ReadFrom(p []byte) (int, net.Addr, error) {
	n, addr, err := c.PacketConn.ReadFrom(p)
	if n > 0 {
		c.readCounter.Add(int64(n))
	}
	return n, addr, err
}

func (c *StatsPacketConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	n, err := c.PacketConn.WriteTo(p, addr)
	if n > 0 {
		c.writeCounter.Add(int64(n))
	}
	return n, err
}
