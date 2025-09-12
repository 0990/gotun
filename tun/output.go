package tun

import (
	"context"
	"errors"
	"github.com/0990/gotun/core"
	"github.com/0990/gotun/pkg/util"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sirupsen/logrus"
	"sync/atomic"
	"time"
)

var (
	connStreamGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name:        "output_idx_get_total",
		Help:        "The total number of processed events",
		ConstLabels: nil,
	}, []string{"idx"})
)

type output interface {
	Run() error
	Close() error
	GetStream() (core.IStream, error)
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

	var makeStream func(addr string, config string) (core.IStream, error)
	var makeStreamMaker func(ctx context.Context, addr string, config string) (core.IStreamMaker, error)

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
	o.autoExpire = extend.AutoExpire
	err = o.CheckCfg()
	if err != nil {
		return nil, err
	}

	return &o, nil
}

type Output struct {
	makeStreamMaker func(ctx context.Context, addr string, config string) (core.IStreamMaker, error)
	makeStream      func(addr string, config string) (core.IStream, error)

	addr   string
	config string

	poolNum    int //此值>0时，会预先生成muxConnNum个连接，用于后续的多路复用，<=0时，每次都会新建连接
	makerPools []*streamMakerContainer
	poolIdx    atomic.Int32

	autoExpire int //此值>0，当makerPool中的连接时间（秒）达到此值时，会重建连接

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
	makers := make([]*streamMakerContainer, p.poolNum)
	for k := range makers {
		m := p.waitCreateStreamMaker()

		w := &streamMakerContainer{}
		w.SetMaker(m, p.autoExpire)
		makers[k] = w
	}
	p.makerPools = makers
	connStreamGauge.Reset()
	return nil
}

// 默认通过makeStream临时创建，不存在makeStream时，则一定从streamMaker池中取streamMaker来创建stream(多路复用情况下)
func (p *Output) GetStream() (core.IStream, error) {
	if p.makeStream != nil {
		return p.makeStream(p.addr, p.config)
	}

	if p.poolNum <= 0 {
		return nil, errors.New("poolnum<=0")
	}

	idx, maker, ok := p.getStreamMaker()
	if ok {
		connStreamGauge.WithLabelValues(util.ToString(idx)).Add(1)
		return maker.OpenStream()
	}

	logrus.Warn("start waitStreamMakerCreated")
	idx, maker, err := p.waitStreamMakerCreated()
	if err == nil {
		connStreamGauge.WithLabelValues(util.ToString(idx)).Add(1)
		return maker.OpenStream()
	}

	logrus.WithError(err).Warn("waitStreamMakerCreated failed")
	return nil, err
}

func (p *Output) getStreamMaker() (int, core.IStreamMaker, bool) {
	// 遍历 pool 找到已有 maker 的 slot
	for i := 0; i < p.poolNum; i++ {
		idx := int(p.poolIdx.Load()) % p.poolNum
		p.poolIdx.Add(1)

		w := p.makerPools[idx]
		if m, ok := w.GetMaker(); ok {
			return idx, m, true
		}

		// 如果没有 maker，就尽量 **只启动一次** 创建任务（TryStartCreate 内部保证原子性）
		// 其它并发者不会重复启动创建 goroutine。
		started := w.TryStartCreate(func() {
			// 创建函数在单独的 goroutine 中执行
			m := p.waitCreateStreamMaker()
			// 无论成功或失败都调用 SetMaker（nil 表示创建失败/关闭等）
			// SetMaker 里应该处理 nil 的安全性
			w.SetMaker(m, p.autoExpire)
		})

		if started {
			connStreamGauge.WithLabelValues(util.ToString(idx)).Set(0)
		}
	}

	return 0, nil, false
}

func (p *Output) waitStreamMakerCreated() (int, core.IStreamMaker, error) {
	now := time.Now()
	for {
		if time.Since(now) > time.Second {
			return -1, nil, errors.New("waitStreamMakerCreated timeout")
		}

		if atomic.LoadInt32(&p.close) > 0 {
			return -1, nil, errors.New("output closed")
		}

		for i := 0; i < p.poolNum; i++ {

			w := p.makerPools[i]

			if m, ok := w.GetMaker(); ok {
				return i, m, nil
			}
		}
		time.Sleep(time.Millisecond * 50)
	}
}

func (p *Output) waitCreateStreamMaker() core.IStreamMaker {

	for {
		logrus.Debug("creating StreamMaker....")
		if conn, err := p.makeStreamMaker(context.Background(), p.addr, p.config); err == nil {
			logrus.Debug("creating StreamMaker ok")
			return conn
		} else {
			//如果关闭了，就不再重连
			if atomic.LoadInt32(&p.close) > 0 {
				return nil
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
