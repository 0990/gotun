package tun

import (
	"context"
	"errors"
	"github.com/sirupsen/logrus"
	"sync/atomic"
	"time"
)

type output interface {
	Run() error
	Close() error
	GetStream() (Stream, error)
}

func NewOutput(output string, config string, extendStr string) (output, error) {
	proto, addr, err := parseProtocol(output)
	if err != nil {
		return nil, err
	}

	extend, err := parseExtend(extendStr)
	if err != nil {
		return nil, err
	}

	var makeStream func(addr string, config string) (Stream, error)
	var makeStreamMaker func(ctx context.Context, addr string, config string) (StreamMaker, error)

	switch proto {
	case TCP:
		makeStream = dialTCP
	case TcpMux:
		makeStreamMaker = dialTCPYamuxBuilder
	case QUIC:
		makeStreamMaker = dialQUICBuilder
	case KCP:
		makeStream = dialKCP
	case KcpMux:
		makeStreamMaker = dialKCPBuilder
	case UDP:
		makeStream = dialUDP
	default:
		return nil, errors.New("unknown protocol")
	}

	var o Output
	o.makeStream = makeStream
	o.makeStreamMaker = makeStreamMaker
	o.addr = addr
	o.config = config
	o.poolNum = extend.MuxConn
	err = o.CheckCfg()
	if err != nil {
		return nil, err
	}

	return &o, nil
}

type Output struct {
	makeStreamMaker func(ctx context.Context, addr string, config string) (StreamMaker, error)
	makeStream      func(addr string, config string) (Stream, error)

	addr   string
	config string

	poolNum    int //此值>0时，会预先生成muxConnNum个连接，用于后续的多路复用，<=0时，每次都会新建连接
	makerPools []StreamMaker
	poolIdx    int

	close int32
}

func (p *Output) CheckCfg() error {
	//下面的stream模式是指，单个连接可以创建多个子连接，而非stream模式下，单个连接仅有一个
	//非stream模式下，单个连接不可复用，所以poolNum无用，而stream模式下，单个连接可以使用mux技术创建新的子连接,所以poolNum有用
	if p.poolNum <= 0 && p.makeStreamMaker != nil {
		return errors.New("stream模式下，预连接数muxConn不可为0")
	}

	if p.poolNum > 0 && p.makeStream != nil {
		return errors.New("非stream模式下，预连接数muxConn值无用，不可大于0")
	}
	return nil
}

func (p *Output) Run() error {
	makers := make([]StreamMaker, p.poolNum)
	for k := range makers {
		m, err := p.waitCreateStreamMaker()
		if err != nil {
			return err
		}
		makers[k] = m
	}
	p.makerPools = makers
	return nil
}

// 默认通过makeStream临时创建，不存在makeStream时，则一定从streamMaker池中取streamMaker来创建stream(多路复用情况下)
func (p *Output) GetStream() (Stream, error) {
	if p.makeStream != nil {
		return p.makeStream(p.addr, p.config)
	}

	if p.poolNum <= 0 {
		return nil, errors.New("poolnum<=0")
	}

	idx := p.poolIdx % p.poolNum
	p.poolIdx++

	maker := p.makerPools[idx]
	if maker == nil || maker.IsClosed() {
		m, err := p.waitCreateStreamMaker()
		if err != nil {
			return nil, errors.New("waitCreateStreamMaker error")
		}
		p.makerPools[idx] = m
	}

	return p.makerPools[idx].OpenStream()
}

func (p *Output) waitCreateStreamMaker() (StreamMaker, error) {

	for {
		logrus.Warn("creating conn....")
		if conn, err := p.makeStreamMaker(context.Background(), p.addr, p.config); err == nil {
			logrus.Warn("creating conn ok")
			return conn, nil
		} else {

			//如果关闭了，就不再重连
			if atomic.LoadInt32(&p.close) > 0 {
				return nil, errors.New("output closed")
			}
			logrus.Warn("re-connecting:", err)
			time.Sleep(time.Second)
		}
	}
}

func (p *Output) Close() error {
	atomic.AddInt32(&p.close, 1)
	for _, v := range p.makerPools {
		err := v.Close()
		if err != nil {
			logrus.WithError(err).Error("Close")
		}
	}
	return nil
}
