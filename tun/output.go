package tun

import (
	"encoding/json"
	"errors"
	"github.com/sirupsen/logrus"
	"time"
)

type output interface {
	Close() error
	GetStream() (Stream, error)
}

func NewOutput(output string, extra string, muxConnCount int) (output, error) {
	proto, addr, err := parseProtocol(output)
	if err != nil {
		return nil, err
	}

	var makeStream func(addr string) (Stream, error)
	var makeStreamMaker func(addr string) (StreamMaker, error)

	switch proto {
	case TCP:
		makeStream = dialTCP
	case TcpMux:
		makeStreamMaker = dialTCPYamuxBuilder
	case QUIC:
		makeStreamMaker = dialQUICBuilder
	case KCP, KcpMux:
		var cfg KCPConfig
		if extra != "" {
			err := json.Unmarshal([]byte(extra), &cfg)
			if err != nil {
				return nil, err
			}
		} else {
			cfg = defaultKCPConfig
		}

		if proto == KCP {
			makeStream = dialKCP(cfg)
		}

		if proto == KcpMux {
			makeStreamMaker = dialKCPBuilder(cfg)
		}
	case UDP:
		makeStream = dialUDP
	default:
		return nil, errors.New("unknown protocol")
	}

	var o Output
	o.makeStream = makeStream
	o.makeStreamMaker = makeStreamMaker
	o.addr = addr
	o.poolNum = muxConnCount
	err = o.Init()
	if err != nil {
		return nil, err
	}

	return &o, nil
}

type Output struct {
	makeStreamMaker func(addr string) (StreamMaker, error)
	makeStream      func(addr string) (Stream, error)

	addr string

	poolNum    int //此值>0时，会预先生成muxConnNum个连接，用于后续的多路复用，<=0时，每次都会新建连接
	makerPools []StreamMaker
	poolIdx    int
}

func (p *Output) Init() error {
	if p.poolNum <= 0 && p.makeStreamMaker != nil {
		return errors.New("stream模式下，预连接数outMuxConn不可为0")
	}

	if p.poolNum > 0 && p.makeStream != nil {
		return errors.New("非stream模式下，预连接数outMuxConn值无用，不可大于0")
	}

	makers := make([]StreamMaker, p.poolNum)
	for k := range makers {
		makers[k] = p.waitCreateStreamMaker()
	}
	p.makerPools = makers
	return nil
}

// 默认通过makeStream临时创建，不存在makeStream时，则一定从streamMaker池中取streamMaker来创建stream(多路复用情况下)
func (p *Output) GetStream() (Stream, error) {
	if p.makeStream != nil {
		return p.makeStream(p.addr)
	}

	if p.poolNum <= 0 {
		return nil, errors.New("poolnum<=0")
	}

	idx := p.poolIdx % p.poolNum
	p.poolIdx++

	maker := p.makerPools[idx]
	if maker == nil || maker.IsClosed() {
		p.makerPools[idx] = p.waitCreateStreamMaker()
	}

	return p.makerPools[idx].OpenStream()
}

func (p *Output) waitCreateStreamMaker() StreamMaker {
	for {
		logrus.Warn("creating conn....")
		if conn, err := p.makeStreamMaker(p.addr); err == nil {
			logrus.Warn("creating conn ok")
			return conn
		} else {
			logrus.Warn("re-connecting:", err)
			time.Sleep(time.Second)
		}
	}
}

func (p *Output) Close() error {
	for _, v := range p.makerPools {
		err := v.Close()
		if err != nil {
			logrus.WithError(err).Error("Close")
		}
	}
	return nil
}
