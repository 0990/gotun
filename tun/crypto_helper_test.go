package tun

import (
	"io"
	"net"
	"testing"
	"time"

	"github.com/0990/gotun/core"
)

type customCopyTestStream struct {
	customCopyCalled bool
}

func (s *customCopyTestStream) ID() string                        { return "custom-copy-test" }
func (s *customCopyTestStream) RemoteAddr() net.Addr              { return nil }
func (s *customCopyTestStream) LocalAddr() net.Addr               { return nil }
func (s *customCopyTestStream) Read(p []byte) (int, error)        { return 0, io.EOF }
func (s *customCopyTestStream) Write(p []byte) (int, error)       { return len(p), nil }
func (s *customCopyTestStream) Close() error                      { return nil }
func (s *customCopyTestStream) SetReadDeadline(t time.Time) error { return nil }

func (s *customCopyTestStream) CustomCopy(in, out core.IStream, id string) error {
	s.customCopyCalled = true
	return nil
}

func Test_PipePreparedPreservesCustomCopy(t *testing.T) {
	helper, err := NewCryptoHelper("", "", "", "", Extend{}, Extend{})
	if err != nil {
		t.Fatal(err)
	}

	srcRaw := &customCopyTestStream{}
	dstRaw := &customCopyTestStream{}

	src, err := helper.WrapSrc(srcRaw)
	if err != nil {
		t.Fatal(err)
	}
	dst, err := helper.WrapDst(dstRaw)
	if err != nil {
		t.Fatal(err)
	}

	helper.PipePrepared(dst, src)

	if !srcRaw.customCopyCalled {
		t.Fatal("expected wrapped source to preserve CustomCopy")
	}
}
