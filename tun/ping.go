package tun

import (
	"encoding/binary"
	"errors"
	"github.com/0990/gotun/server/socks5client"
	"github.com/0990/socks5"
	probing "github.com/prometheus-community/pro-bing"
	"log"
	"net"
	"time"
)

type IPing interface {
	Ping(count int, interval time.Duration) (int, error)
}

func NewPing(typ string, addr string) (IPing, error) {
	switch typ {
	case "ping":
		return &Ping{addr: addr}, nil
	case "tcp_ack":
		return &TcpAckPing{addr: addr}, nil
	case "socks5_ack":
		return &Socks5AckPing{addr: addr}, nil
	default:
		return nil, errors.New("unknown protocol")
	}
}

type Socks5AckPing struct {
	addr string
}

func (p *Socks5AckPing) Ping(count int, interval time.Duration) (int, error) {
	clientCfg, testUrl, err := socks5client.ParseUrl(p.addr)
	if err != nil {
		return 0, err
	}
	sc := socks5.NewSocks5Client(clientCfg)
	conn, err := sc.Dial("tcp", testUrl)
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	return doConnPing(conn, count, interval)
}

type TcpAckPing struct {
	addr string
}

func (p *TcpAckPing) Ping(count int, interval time.Duration) (int, error) {
	conn, err := net.Dial("tcp", p.addr)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	return doConnPing(conn, count, interval)
}

func doConnPing(conn net.Conn, count int, interval time.Duration) (int, error) {
	var total uint64
	for i := 0; i < count; i++ {
		// 获取当前时间戳
		timestamp := time.Now().UnixMilli()
		message := make([]byte, 8) // 8 字节用于存储时间戳
		binary.BigEndian.PutUint64(message, uint64(timestamp))
		_, err := conn.Write(message)
		if err != nil {
			log.Fatal(err)
		}
		buf := make([]byte, 8) // 读取 8 字节的时间戳
		conn.SetReadDeadline(time.Now().Add(time.Second))
		n, err := conn.Read(buf)
		if err != nil {
			return 0, err
		}
		returnedTimestamp := binary.BigEndian.Uint64(buf[:n])
		roundTripTime := uint64(time.Now().UnixMilli()) - uint64(returnedTimestamp)
		total += roundTripTime
		time.Sleep(interval)
	}
	return int(total) / count, nil
}

type Ping struct {
	addr string
}

func (p *Ping) Ping(count int, interval time.Duration) (int, error) {
	pinger, err := probing.NewPinger(p.addr)
	pinger.Interval = interval
	pinger.Timeout = 1 * time.Second
	if err != nil {
		return 0, err
	}
	pinger.Count = count
	err = pinger.Run() // Blocks until finished.
	if err != nil {
		return 0, err
	}
	stats := pinger.Statistics()
	return int(stats.AvgRtt.Milliseconds()), nil
}
