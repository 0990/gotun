package tun

import (
	"errors"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

type ProbeChannel struct {
	out    output
	helper *CryptoHelper

	mu      sync.RWMutex
	current *FrameStream
	close   chan struct{}
}

func NewProbeChannel(out output, helper *CryptoHelper) *ProbeChannel {
	return &ProbeChannel{
		out:    out,
		helper: helper,
		close:  make(chan struct{}),
	}
}

func (c *ProbeChannel) Run() {
	go c.loop()
}

func (c *ProbeChannel) Close() {
	select {
	case <-c.close:
		return
	default:
		close(c.close)
	}
	c.clearCurrent(nil)
}

func (c *ProbeChannel) Probe(timeout time.Duration) (time.Duration, error) {
	stream := c.currentStream()
	if stream == nil {
		return 0, errors.New("no active probe stream")
	}
	rtt, err := stream.Probe(timeout)
	if err != nil {
		c.clearCurrent(stream)
		return 0, err
	}
	return rtt, nil
}

func (c *ProbeChannel) loop() {
	for {
		select {
		case <-c.close:
			return
		default:
		}

		stream, err := c.openStream()
		if err != nil {
			logrus.WithError(err).Debug("open probe stream")
			if !c.sleepOrClose(time.Second) {
				return
			}
			continue
		}

		c.setCurrent(stream)
		err = stream.ServeControlLoop()
		c.clearCurrent(stream)
		if err != nil && !errors.Is(err, ErrProbeChannelClosed) {
			logrus.WithError(err).Debug("probe stream closed")
		}
	}
}

func (c *ProbeChannel) openStream() (*FrameStream, error) {
	raw, err := c.out.GetProbeStream()
	if err != nil {
		return nil, err
	}

	wrapped, err := c.helper.WrapProbeDst(raw)
	if err != nil {
		_ = raw.Close()
		return nil, err
	}

	frameStream, ok := wrapped.(*FrameStream)
	if !ok {
		_ = wrapped.Close()
		return nil, errors.New("probe stream is not frame stream")
	}
	return frameStream, nil
}

func (c *ProbeChannel) currentStream() *FrameStream {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.current
}

func (c *ProbeChannel) setCurrent(stream *FrameStream) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.current != nil && c.current != stream {
		_ = c.current.Close()
	}
	c.current = stream
}

func (c *ProbeChannel) clearCurrent(stream *FrameStream) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if stream != nil && c.current != stream {
		return
	}
	if c.current != nil {
		_ = c.current.Close()
	}
	c.current = nil
}

func (c *ProbeChannel) sleepOrClose(d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-c.close:
		return false
	}
}
