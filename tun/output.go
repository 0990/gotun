package tun

import (
	"context"
	"errors"
	"github.com/0990/gotun/core"
	"github.com/0990/gotun/pkg/stats"
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

	openStreamHistogram = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:        "open_stream_duration_seconds",
		Help:        "Time taken to open stream.",
		Buckets:     []float64{0.01, 0.05, 0.2, 0.5, 2, 4, 10},
		ConstLabels: nil,
	}, []string{"status"})
)

type output interface {
	Run() error
	Close() error
	GetStream() (core.IStream, error)
	QualitySnapshot() QualitySnapshot
	QualitySummary() QualitySummary
	FrameHeaderEnabled() bool
	ProbeConfig() Extend
}

type makeStreamFunc func(addr string, config string, readCounter, writeCounter stats.Counter) (core.IStream, error)
type makeStreamMakerFunc = func(ctx context.Context, addr string, config string, readCounter, writeCounter stats.Counter) (core.IStreamMaker, error)

func NewOutput(name string, output string, config string, extendStr string, readCounter, writeCounter stats.Counter) (output, error) {
	proto, addr, err := parseProtocol(output)
	if err != nil {
		return nil, err
	}

	extend, err := parseExtend(extendStr)
	if err != nil {
		return nil, err
	}

	var makeStream makeStreamFunc
	var makeStreamMaker makeStreamMakerFunc

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
	case KCPX:
		makeStream = dialKCPX
	case KCPXMux:
		makeStreamMaker = dialKCPXBuilder
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
	o.readCounter = readCounter
	o.writeCounter = writeCounter
	o.name = name
	o.extend = extend
	o.quality = NewQualityTracker(name, addr, readCounter, writeCounter, extend.FrameHeaderEnable, extend.ProbeWindowSize)
	err = o.CheckCfg()
	if err != nil {
		return nil, err
	}

	return &o, nil
}

type Output struct {
	makeStreamMaker makeStreamMakerFunc
	makeStream      makeStreamFunc

	addr   string
	config string
	name   string
	extend Extend

	poolNum    int //此值>0时，会预先生成muxConnNum个连接，用于后续的多路复用，<=0时，每次都会新建连接
	makerPools []*streamMakerContainer
	poolIdx    atomic.Int32

	autoExpire int //此值>0，当makerPool中的连接时间（秒）达到此值时，会重建连接

	close int32

	readCounter  stats.Counter
	writeCounter stats.Counter
	quality      *QualityTracker
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
	now := time.Now()
	status := ""
	defer func() {
		duration := time.Since(now).Seconds()
		openStreamHistogram.WithLabelValues(status).Observe(duration)
	}()

	if p.makeStream != nil {
		status = "makeStream"
		stream, err := p.makeStream(p.addr, p.config, p.readCounter, p.writeCounter)
		if err != nil {
			p.quality.RecordOpenFailure()
			return nil, err
		}
		p.quality.RecordOpenSuccess()
		return p.wrapQualityStream(stream), nil
	}

	if p.poolNum <= 0 {
		return nil, errors.New("poolnum<=0")
	}

	idx, maker, ok := p.getStreamMaker()
	if ok {
		status = "openStream"
		connStreamGauge.WithLabelValues(util.ToString(idx)).Add(1)
		stream, err := maker.OpenStream()
		if err != nil {
			p.quality.RecordOpenFailure()
			return nil, err
		}
		p.quality.RecordOpenSuccess()
		return p.wrapQualityStream(stream), nil
	}

	logrus.Warn("start waitStreamMakerCreated")
	idx, maker, err := p.waitStreamMakerCreated()
	if err == nil {
		status = "waitMaker"
		connStreamGauge.WithLabelValues(util.ToString(idx)).Add(1)
		stream, openErr := maker.OpenStream()
		if openErr != nil {
			p.quality.RecordOpenFailure()
			return nil, openErr
		}
		p.quality.RecordOpenSuccess()
		return p.wrapQualityStream(stream), nil
	}

	status = "returnError"
	logrus.WithError(err).Warn("waitStreamMakerCreated failed")
	p.quality.RecordOpenFailure()
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
		if conn, err := p.makeStreamMaker(context.Background(), p.addr, p.config, p.readCounter, p.writeCounter); err == nil {
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

func (p *Output) wrapQualityStream(stream core.IStream) core.IStream {
	p.quality.RecordStreamOpen()
	return &qualityStream{IStream: stream, tracker: p.quality}
}

func (p *Output) QualitySnapshot() QualitySnapshot {
	return p.quality.Snapshot()
}

func (p *Output) QualitySummary() QualitySummary {
	return p.quality.Summary()
}

func (p *Output) FrameHeaderEnabled() bool {
	return p.extend.FrameHeaderEnable
}

func (p *Output) ProbeConfig() Extend {
	return p.extend
}

type qualityStream struct {
	core.IStream
	tracker *QualityTracker
	closed  atomic.Bool
}

func (s *qualityStream) Close() error {
	if s.closed.CompareAndSwap(false, true) {
		s.tracker.RecordStreamClose()
	}
	return s.IStream.Close()
}

func outputTracker(out output) *QualityTracker {
	if concrete, ok := out.(*Output); ok {
		return concrete.quality
	}
	return NewQualityTracker("", "", nil, nil, false, 0)
}
