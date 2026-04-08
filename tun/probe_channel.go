package tun

import (
	"errors"
	"time"
)

type ProbeChannel struct {
	out    output
	helper *CryptoHelper

	close chan struct{}
}

func NewProbeChannel(out output, helper *CryptoHelper) *ProbeChannel {
	return &ProbeChannel{
		out:    out,
		helper: helper,
		close:  make(chan struct{}),
	}
}

func (c *ProbeChannel) Run() {
	// Probes now use a short-lived dedicated stream per request.
	// Keep Run for API compatibility with existing startup code.
}

func (c *ProbeChannel) Close() {
	select {
	case <-c.close:
		return
	default:
		close(c.close)
	}
}

func (c *ProbeChannel) Probe(timeout time.Duration) (time.Duration, error) {
	select {
	case <-c.close:
		return 0, ErrProbeChannelClosed
	default:
	}

	stream, err := c.openStream()
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = stream.Close()
	}()

	// Each probe owns its own control loop so the measured RTT is not pinned to
	// a long-lived mux substream created during startup.
	go func() {
		_ = stream.ServeControlLoop()
	}()

	rtt, err := stream.Probe(timeout)
	if err != nil {
		return 0, err
	}
	return rtt, nil
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
