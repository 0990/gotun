package tun

import (
	"crypto/cipher"
	"errors"
	"io"
	"net"
	"time"

	"github.com/0990/gotun/core"
	"github.com/0990/gotun/pkg/crypto"
	"github.com/0990/gotun/pkg/util"
	"github.com/sirupsen/logrus"
)

type CryptoHelper struct {
	srcMode, dstMode crypto.Mode
	srcAead, dstAead cipher.AEAD
	srcExtend        Extend
	dstExtend        Extend
}

func NewCryptoHelperWithConfig(config Config) (*CryptoHelper, error) {
	srcExtend, _ := parseExtend(config.InExtend)
	dstExtend, _ := parseExtend(config.OutExtend)
	return NewCryptoHelper(config.InDecryptMode, config.InDecryptKey, config.OutCryptMode, config.OutCryptKey, srcExtend, dstExtend)
}

func NewCryptoHelper(inDecryptMode, inDecryptKey, outDecryptMode, outDecryptKey string, srcExtend, dstExtend Extend) (*CryptoHelper, error) {
	srcMode, err := crypto.ToMode(inDecryptMode)
	if err != nil {
		return nil, err
	}
	dstMode, err := crypto.ToMode(outDecryptMode)
	if err != nil {
		return nil, err
	}

	srcAead, err := util.CreateAesGcmAead(util.StringToAesKey(inDecryptKey, 32))
	if err != nil {
		return nil, err
	}

	dstAead, err := util.CreateAesGcmAead(util.StringToAesKey(outDecryptKey, 32))
	if err != nil {
		return nil, err
	}

	return &CryptoHelper{
		srcMode:   srcMode,
		dstMode:   dstMode,
		srcAead:   srcAead,
		dstAead:   dstAead,
		srcExtend: srcExtend,
		dstExtend: dstExtend,
	}, nil
}

func (c *CryptoHelper) Pipe(dst, src core.IStream) {
	srcWrapped, err := c.WrapSrc(src)
	if err != nil {
		return
	}
	dstWrapped, err := c.WrapDst(dst)
	if err != nil {
		return
	}
	c.PipePrepared(dstWrapped, srcWrapped)
}

func (c *CryptoHelper) PipePrepared(dst, src core.IStream) {
	srcLocalAddr := src.LocalAddr()
	srcRemoteAddr := src.RemoteAddr()
	dstLocalAddr := dst.LocalAddr()
	dstRemoteAddr := dst.RemoteAddr()

	id := util.TraceID(4)
	log := logrus.WithFields(logrus.Fields{
		"inLocal":   srcLocalAddr,
		"inRemote":  srcRemoteAddr,
		"in":        src.ID(),
		"outLocal":  dstLocalAddr,
		"outRemote": dstRemoteAddr,
		"out":       dst.ID(),
		"id":        id,
	})

	now := time.Now()
	log.Debug("stream opened")
	closeReason := "eof"
	defer func() {
		log.WithFields(logrus.Fields{
			"duration": int(time.Since(now).Seconds()),
			"reason":   closeReason,
		}).Debug("stream closed")
	}()

	err := c.pipePrepared(dst, src, id)
	if err != nil {
		if !errors.Is(err, io.EOF) {
			closeReason = err.Error()
			//log.WithError(err).Debug("pipe end")
		}
	}
}

func (c *CryptoHelper) pipePrepared(dst, src core.IStream, id string) error {
	if h, ok := findCustomCopy(src); ok {
		err := h.CustomCopy(src, dst, id)
		if err != nil {
			return err
		}
		return nil
	}

	return core.Pipe(src, dst, time.Second*300)
}

func findCustomCopy(stream core.IStream) (CustomCopy, bool) {
	switch s := stream.(type) {
	case *FrameStream:
		return findCustomCopy(s.stream)
	case *CryptoStream:
		return findCustomCopy(s.stream)
	default:
		h, ok := stream.(CustomCopy)
		return h, ok
	}
}

func (c *CryptoHelper) SrcReaderWriter(rw io.ReadWriter) (io.ReadWriter, error) {
	return crypto.NewReaderWriter(rw, c.srcMode, c.srcAead)
}

func (c *CryptoHelper) DstReaderWriter(rw io.ReadWriter) (io.ReadWriter, error) {
	return crypto.NewReaderWriter(rw, c.dstMode, c.dstAead)
}

type CryptoStream struct {
	rw     io.ReadWriter
	stream core.IStream
}

func (p *CryptoStream) SetReadDeadline(t time.Time) error {
	return p.stream.SetReadDeadline(t)
}

func (p *CryptoStream) Close() error {
	return p.stream.Close()
}

func (p *CryptoStream) Read(b []byte) (int, error) {
	return p.rw.Read(b)
}

func (p *CryptoStream) Write(b []byte) (int, error) {
	return p.rw.Write(b)
}

func (p *CryptoStream) ID() string {
	return p.stream.ID()
}

func (p *CryptoStream) LocalAddr() net.Addr {
	return p.stream.LocalAddr()
}

func (p *CryptoStream) RemoteAddr() net.Addr {
	return p.stream.RemoteAddr()
}

func (c *CryptoHelper) SrcCrypto(s core.IStream) (core.IStream, error) {
	rw, err := crypto.NewReaderWriter(s, c.srcMode, c.srcAead)
	if err != nil {
		return nil, err
	}

	return &CryptoStream{
		rw:     rw,
		stream: s,
	}, nil
}

func (c *CryptoHelper) DstCrypto(s core.IStream) (core.IStream, error) {
	rw, err := crypto.NewReaderWriter(s, c.dstMode, c.dstAead)
	if err != nil {
		return nil, err
	}

	return &CryptoStream{
		rw:     rw,
		stream: s,
	}, nil
}

func (c *CryptoHelper) WrapSrc(s core.IStream) (core.IStream, error) {
	stream, err := c.SrcCrypto(s)
	if err != nil {
		return nil, err
	}
	if !c.srcExtend.FrameHeaderEnable {
		return stream, nil
	}
	return NewFrameStream(stream), nil
}

func (c *CryptoHelper) WrapDst(s core.IStream) (core.IStream, error) {
	stream, err := c.DstCrypto(s)
	if err != nil {
		return nil, err
	}
	if !c.dstExtend.FrameHeaderEnable {
		return stream, nil
	}
	return NewFrameStream(stream), nil
}

func (c *CryptoHelper) SrcFrameEnabled() bool {
	return c.srcExtend.FrameHeaderEnable
}

func (c *CryptoHelper) DstFrameEnabled() bool {
	return c.dstExtend.FrameHeaderEnable
}

func (c *CryptoHelper) DstProbeConfig() Extend {
	return c.dstExtend
}
