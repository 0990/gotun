package tun

import (
	"encoding/binary"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/0990/gotun/core"
)

const (
	BandwidthStatusDisabled = "disabled"
	BandwidthStatusIdle     = "idle"
	BandwidthStatusOK       = "ok"
	BandwidthStatusError    = "error"
)

const (
	bandwidthModeDownload byte = 0x01

	bandwidthTestDuration = 10 * time.Second
	bandwidthChunkSize    = 32 * 1024
)

type BandwidthSummary struct {
	Status    string    `json:"status"`
	Mbps      float64   `json:"mbps"`
	LastError string    `json:"last_error"`
	TestedAt  time.Time `json:"tested_at"`
}

type BandwidthTracker struct {
	mu      sync.RWMutex
	enabled bool
	summary BandwidthSummary
}

type bandwidthRequest struct {
	Mode       byte
	DurationMS uint32
	ChunkSize  uint32
}

func NewBandwidthTracker(enabled bool) *BandwidthTracker {
	status := BandwidthStatusDisabled
	if enabled {
		status = BandwidthStatusIdle
	}
	return &BandwidthTracker{
		enabled: enabled,
		summary: BandwidthSummary{Status: status},
	}
}

func (t *BandwidthTracker) Summary() BandwidthSummary {
	if t == nil {
		return BandwidthSummary{Status: BandwidthStatusDisabled}
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.summary
}

func (t *BandwidthTracker) Store(summary BandwidthSummary) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.summary = summary
}

func (t *BandwidthTracker) DisabledSummary(msg string) BandwidthSummary {
	summary := BandwidthSummary{
		Status:    BandwidthStatusDisabled,
		LastError: msg,
		TestedAt:  time.Now(),
	}
	t.Store(summary)
	return summary
}

func (t *BandwidthTracker) ErrorSummary(err error) BandwidthSummary {
	summary := BandwidthSummary{
		Status:   BandwidthStatusError,
		TestedAt: time.Now(),
	}
	if err != nil {
		summary.LastError = err.Error()
	}
	t.Store(summary)
	return summary
}

func RunBandwidthTest(out output, helper *CryptoHelper) (BandwidthSummary, error) {
	if out == nil || helper == nil {
		return BandwidthSummary{Status: BandwidthStatusError, LastError: "bandwidth test not ready"}, errors.New("bandwidth test not ready")
	}
	if !out.FrameHeaderEnabled() {
		return BandwidthSummary{Status: BandwidthStatusDisabled, LastError: "frame_header_enable disabled"}, nil
	}

	stream, err := openBandwidthStream(out, helper)
	if err != nil {
		return BandwidthSummary{Status: BandwidthStatusError, LastError: err.Error(), TestedAt: time.Now()}, err
	}
	defer func() {
		_ = stream.Close()
	}()

	req := bandwidthRequest{
		Mode:       bandwidthModeDownload,
		DurationMS: uint32(bandwidthTestDuration.Milliseconds()),
		ChunkSize:  bandwidthChunkSize,
	}
	if err := writeBandwidthRequest(stream, req); err != nil {
		return BandwidthSummary{Status: BandwidthStatusError, LastError: err.Error(), TestedAt: time.Now()}, err
	}

	start := time.Now()
	var totalBytes int64
	buf := make([]byte, req.ChunkSize)
	for {
		n, readErr := stream.Read(buf)
		totalBytes += int64(n)
		if readErr == nil {
			continue
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		return BandwidthSummary{Status: BandwidthStatusError, LastError: readErr.Error(), TestedAt: time.Now()}, readErr
	}

	elapsed := time.Since(start)
	if elapsed <= 0 {
		elapsed = time.Millisecond
	}

	mbps := float64(totalBytes*8) / elapsed.Seconds() / 1000 / 1000
	return BandwidthSummary{
		Status:   BandwidthStatusOK,
		Mbps:     mbps,
		TestedAt: time.Now(),
	}, nil
}

func ServeBandwidthLoop(stream *FrameStream) error {
	req, err := readBandwidthRequest(stream)
	if err != nil {
		return err
	}

	switch req.Mode {
	case bandwidthModeDownload:
		return serveBandwidthDownload(stream, req)
	default:
		return errors.New("unsupported bandwidth mode")
	}
}

func openBandwidthStream(out output, helper *CryptoHelper) (*FrameStream, error) {
	raw, err := out.GetBandwidthStream()
	if err != nil {
		return nil, err
	}

	wrapped, err := helper.WrapBandwidthDst(raw)
	if err != nil {
		_ = raw.Close()
		return nil, err
	}

	frameStream, ok := wrapped.(*FrameStream)
	if !ok {
		_ = wrapped.Close()
		return nil, errors.New("bandwidth stream is not frame stream")
	}
	return frameStream, nil
}

func serveBandwidthDownload(stream *FrameStream, req bandwidthRequest) error {
	if req.ChunkSize == 0 || req.ChunkSize > core.MaxSegmentSize {
		return errors.New("invalid bandwidth chunk size")
	}
	if req.DurationMS == 0 {
		return errors.New("invalid bandwidth duration")
	}

	buf := make([]byte, req.ChunkSize)
	deadline := time.Now().Add(time.Duration(req.DurationMS) * time.Millisecond)
	for time.Now().Before(deadline) {
		if _, err := stream.Write(buf); err != nil {
			return err
		}
	}
	return nil
}

func writeBandwidthRequest(w io.Writer, req bandwidthRequest) error {
	buf := make([]byte, 9)
	buf[0] = req.Mode
	binary.BigEndian.PutUint32(buf[1:5], req.DurationMS)
	binary.BigEndian.PutUint32(buf[5:9], req.ChunkSize)
	_, err := w.Write(buf)
	return err
}

func readBandwidthRequest(r io.Reader) (bandwidthRequest, error) {
	buf := make([]byte, 9)
	if _, err := io.ReadFull(r, buf); err != nil {
		return bandwidthRequest{}, err
	}
	return bandwidthRequest{
		Mode:       buf[0],
		DurationMS: binary.BigEndian.Uint32(buf[1:5]),
		ChunkSize:  binary.BigEndian.Uint32(buf[5:9]),
	}, nil
}
