package tun

import (
	"crypto/cipher"
	"errors"
	"github.com/0990/gotun/core"
	"github.com/0990/gotun/pkg/crypto"
	"github.com/0990/gotun/pkg/util"
	"github.com/sirupsen/logrus"
	"io"
	"net"
	"time"
)

type CryptoHelper struct {
	srcMode, dstMode crypto.Mode
	srcAead, dstAead cipher.AEAD
}

func NewCryptoHelperWithConfig(config Config) (*CryptoHelper, error) {
	return NewCryptoHelper(config.InDecryptMode, config.InDecryptKey, config.OutCryptMode, config.OutCryptKey)
}

func NewCryptoHelper(inDecryptMode, inDecryptKey, outDecryptMode, outDecryptKey string) (*CryptoHelper, error) {
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
		srcMode: srcMode,
		dstMode: dstMode,
		srcAead: srcAead,
		dstAead: dstAead,
	}, nil
}

func (c *CryptoHelper) Pipe(dst, src core.IStream) {
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

	log.Debug("stream opened")
	defer log.Debug("stream closed")

	err := c.pipe(dst, src, id)
	if err != nil {
		if !errors.Is(err, io.EOF) {
			log.WithError(err).Debug("pipe end")
		}
	}
}

func (c *CryptoHelper) pipe(dst, src core.IStream, id string) error {
	s, err := crypto.NewReaderWriter(src, c.srcMode, c.srcAead)
	if err != nil {
		return err
	}
	d, err := crypto.NewReaderWriter(dst, c.dstMode, c.dstAead)
	if err != nil {
		return err
	}

	in := &CryptoStream{
		rw:     s,
		stream: src,
	}

	out := &CryptoStream{
		rw:     d,
		stream: dst,
	}

	if h, ok := src.(CustomCopy); ok {
		err := h.CustomCopy(in, out, id)
		if err != nil {
			return err
		}
		return nil
	}

	return core.Pipe(in, out, time.Second*60)
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
