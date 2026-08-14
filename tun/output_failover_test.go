package tun

import (
	"bytes"
	"io"
	"net"
	"testing"
	"time"

	"github.com/0990/gotun/core"
	"github.com/0990/gotun/pkg/stats"
)

// captureStream 记录写入口，用于断言 WrapDst 是否套用了帧头握手
type captureStream struct {
	buf bytes.Buffer
}

func (s *captureStream) ID() string                        { return "capture" }
func (s *captureStream) RemoteAddr() net.Addr              { return nil }
func (s *captureStream) LocalAddr() net.Addr               { return nil }
func (s *captureStream) Read(p []byte) (int, error)        { return 0, io.EOF }
func (s *captureStream) Write(p []byte) (int, error)       { return s.buf.Write(p) }
func (s *captureStream) Close() error                      { return nil }
func (s *captureStream) SetReadDeadline(t time.Time) error { return nil }

// newFrameHeaderMember 构造一个 output 开启帧头的 *Server member，
// 其 output 拨号被替换为可注入的桩（不真正拨号）。
// 返回该 Server 与注入拨号得到的底层流，便于断言写入了帧头。
func newFrameHeaderMember(t *testing.T, name string, dial func() (core.IStream, error)) (*Server, **captureStream) {
	t.Helper()
	cfg := Config{
		Name:         name,
		Input:        "tcp@127.0.0.1:0",
		Output:       "tcp@127.0.0.1:0",
		OutCryptMode: "none",
		OutExtend:    `{"frame_header_enable":true}`,
	}
	srv, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer 失败: %v", err)
	}
	var raw *captureStream
	out := srv.output.(*Output)
	out.makeStream = func(addr, config string, rc, wc stats.Counter) (core.IStream, error) {
		s, err := dial()
		if err != nil {
			return nil, err
		}
		raw = s.(*captureStream)
		return s, nil
	}
	srv.SetStatus("running")                         // memberAlive 要求 running 才参与选址
	out.quality.RecordProbeSuccess(time.Millisecond) // 帧头开启时空样本为 down，补一条成功样本使其可选
	return srv, &raw
}

// TestFailoverOutput_GetStreamWithCrypto 验证出口模式借用 member output 时，
// 返回的 CryptoHelper 是该 member 自己的（带帧头），而非路由的空加密。
func TestFailoverOutput_GetStreamWithCrypto(t *testing.T) {
	srv, raw := newFrameHeaderMember(t, "m1", func() (core.IStream, error) { return &captureStream{}, nil })
	m := newMgrWithServices(srv)
	sel := newRouteSelector(m, []string{"m1"})
	fo := newFailoverOutput("r", sel)

	stream, ch, err := fo.GetStreamWithCrypto()
	if err != nil {
		t.Fatalf("GetStreamWithCrypto 失败: %v", err)
	}
	if ch == nil {
		t.Fatal("应返回 member 的 CryptoHelper，得到 nil")
	}
	if ch != srv.cryptoHelper {
		t.Fatal("返回的应是所选 member 自己的 cryptoHelper")
	}
	if !ch.DstFrameEnabled() {
		t.Fatal("member 的 output 侧应开启帧头")
	}

	// 用返回的 helper 包裹 dst，应向底层流写入帧头握手（GTH1...），证明用的是 member 的加密配置
	if _, err := ch.WrapDst(stream); err != nil {
		t.Fatalf("WrapDst 失败: %v", err)
	}
	if !bytes.HasPrefix((*raw).buf.Bytes(), streamHandshakeMagic[:]) {
		t.Fatalf("WrapDst 应向底层流写入帧头握手, 实际: %v", (*raw).buf.Bytes())
	}
}

// TestFailoverOutput_GetStreamWithCrypto_Failover 验证顺延到下一个 member 时，
// 流和加密助手都来自下一个 member。
func TestFailoverOutput_GetStreamWithCrypto_Failover(t *testing.T) {
	bad, _ := newFrameHeaderMember(t, "bad", func() (core.IStream, error) {
		return nil, errStubDial
	})
	good, _ := newFrameHeaderMember(t, "good", func() (core.IStream, error) { return &captureStream{}, nil })
	m := newMgrWithServices(bad, good)
	sel := newRouteSelector(m, []string{"bad", "good"})
	fo := newFailoverOutput("r", sel)

	_, ch, err := fo.GetStreamWithCrypto()
	if err != nil {
		t.Fatalf("顺延后应成功: %v", err)
	}
	if ch != good.cryptoHelper {
		t.Fatal("顺延到 good 后应返回 good 的 cryptoHelper")
	}
}

var errStubDial = &stubDialError{}

type stubDialError struct{}

func (e *stubDialError) Error() string { return "stub dial fail" }
