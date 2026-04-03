package tun

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0990/gotun/core"
)

const (
	frameTypeBusiness  byte = 0x00
	frameTypeProbeReq  byte = 0x01
	frameTypeProbeResp byte = 0x02
)

type ProbeReq struct {
	Seq          uint64
	SentUnixNano int64
}

type ProbeResp struct {
	Seq          uint64
	SentUnixNano int64
	RecvUnixNano int64
}

type FrameStream struct {
	stream core.IStream

	pending []byte
	writeMu sync.Mutex

	probeSeq     atomic.Uint64
	probeMu      sync.Mutex
	waitersMu    sync.Mutex
	probeWaiters map[uint64]chan ProbeResp
}

func NewFrameStream(stream core.IStream) *FrameStream {
	return &FrameStream{
		stream:       stream,
		probeWaiters: make(map[uint64]chan ProbeResp),
	}
}

func (s *FrameStream) ID() string {
	return s.stream.ID()
}

func (s *FrameStream) RemoteAddr() net.Addr {
	return s.stream.RemoteAddr()
}

func (s *FrameStream) LocalAddr() net.Addr {
	return s.stream.LocalAddr()
}

func (s *FrameStream) SetReadDeadline(t time.Time) error {
	return s.stream.SetReadDeadline(t)
}

func (s *FrameStream) Close() error {
	return s.stream.Close()
}

func (s *FrameStream) Read(p []byte) (int, error) {
	if len(s.pending) > 0 {
		n := copy(p, s.pending)
		s.pending = s.pending[n:]
		return n, nil
	}

	for {
		frameType, payload, err := readFrame(s.stream)
		if err != nil {
			return 0, err
		}
		switch frameType {
		case frameTypeBusiness:
			n := copy(p, payload)
			if n < len(payload) {
				s.pending = append(s.pending[:0], payload[n:]...)
			}
			return n, nil
		case frameTypeProbeReq:
			req, err := decodeProbeReq(payload)
			if err != nil {
				return 0, err
			}
			resp := ProbeResp{
				Seq:          req.Seq,
				SentUnixNano: req.SentUnixNano,
				RecvUnixNano: time.Now().UnixNano(),
			}
			if err := s.WriteProbeResp(resp); err != nil {
				return 0, err
			}
		case frameTypeProbeResp:
			resp, err := decodeProbeResp(payload)
			if err != nil {
				return 0, err
			}
			s.notifyProbeWaiter(resp)
		default:
			return 0, errors.New("unknown frame type")
		}
	}
}

func (s *FrameStream) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if err := s.writeFrame(frameTypeBusiness, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (s *FrameStream) WriteProbeReq(req ProbeReq) error {
	return s.writeFrame(frameTypeProbeReq, encodeProbeReq(req))
}

func (s *FrameStream) WriteProbeResp(resp ProbeResp) error {
	return s.writeFrame(frameTypeProbeResp, encodeProbeResp(resp))
}

func (s *FrameStream) Probe(timeout time.Duration) (time.Duration, error) {
	s.probeMu.Lock()
	defer s.probeMu.Unlock()

	seq := s.probeSeq.Add(1)
	respCh := make(chan ProbeResp, 1)
	s.registerProbeWaiter(seq, respCh)
	defer s.unregisterProbeWaiter(seq)

	start := time.Now()
	req := ProbeReq{
		Seq:          seq,
		SentUnixNano: start.UnixNano(),
	}
	if err := s.WriteProbeReq(req); err != nil {
		return 0, err
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case resp := <-respCh:
		if resp.Seq != seq {
			return 0, errors.New("probe sequence mismatch")
		}
		return time.Since(time.Unix(0, resp.SentUnixNano)), nil
	case <-timer.C:
		return 0, errors.New("probe timeout")
	}
}

func (s *FrameStream) writeFrame(frameType byte, payload []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return writeFrame(s.stream, frameType, payload)
}

func writeFrame(w io.Writer, frameType byte, payload []byte) error {
	if len(payload) > core.MaxSegmentSize {
		return errors.New("frame payload too large")
	}
	buf := make([]byte, 3+len(payload))
	buf[0] = frameType
	binary.BigEndian.PutUint16(buf[1:3], uint16(len(payload)))
	copy(buf[3:], payload)
	_, err := w.Write(buf)
	return err
}

func (s *FrameStream) registerProbeWaiter(seq uint64, ch chan ProbeResp) {
	s.waitersMu.Lock()
	defer s.waitersMu.Unlock()
	s.probeWaiters[seq] = ch
}

func (s *FrameStream) unregisterProbeWaiter(seq uint64) {
	s.waitersMu.Lock()
	defer s.waitersMu.Unlock()
	delete(s.probeWaiters, seq)
}

func (s *FrameStream) notifyProbeWaiter(resp ProbeResp) {
	s.waitersMu.Lock()
	ch, ok := s.probeWaiters[resp.Seq]
	s.waitersMu.Unlock()
	if !ok {
		return
	}
	select {
	case ch <- resp:
	default:
	}
}

func readFrame(r io.Reader) (byte, []byte, error) {
	header := make([]byte, 3)
	if _, err := io.ReadFull(r, header); err != nil {
		return 0, nil, err
	}
	size := int(binary.BigEndian.Uint16(header[1:3]))
	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	return header[0], payload, nil
}

func encodeProbeReq(req ProbeReq) []byte {
	buf := make([]byte, 16)
	binary.BigEndian.PutUint64(buf[0:8], req.Seq)
	binary.BigEndian.PutUint64(buf[8:16], uint64(req.SentUnixNano))
	return buf
}

func decodeProbeReq(payload []byte) (ProbeReq, error) {
	if len(payload) != 16 {
		return ProbeReq{}, errors.New("invalid probe request payload")
	}
	return ProbeReq{
		Seq:          binary.BigEndian.Uint64(payload[0:8]),
		SentUnixNano: int64(binary.BigEndian.Uint64(payload[8:16])),
	}, nil
}

func encodeProbeResp(resp ProbeResp) []byte {
	buf := make([]byte, 24)
	binary.BigEndian.PutUint64(buf[0:8], resp.Seq)
	binary.BigEndian.PutUint64(buf[8:16], uint64(resp.SentUnixNano))
	binary.BigEndian.PutUint64(buf[16:24], uint64(resp.RecvUnixNano))
	return buf
}

func decodeProbeResp(payload []byte) (ProbeResp, error) {
	if len(payload) != 24 {
		return ProbeResp{}, errors.New("invalid probe response payload")
	}
	return ProbeResp{
		Seq:          binary.BigEndian.Uint64(payload[0:8]),
		SentUnixNano: int64(binary.BigEndian.Uint64(payload[8:16])),
		RecvUnixNano: int64(binary.BigEndian.Uint64(payload[16:24])),
	}, nil
}
