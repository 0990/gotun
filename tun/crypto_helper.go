package tun

import (
	"crypto/cipher"
	"github.com/0990/gotun/pkg/crypto"
	"github.com/0990/gotun/pkg/util"
	"github.com/sirupsen/logrus"
	"io"
	"time"
)

type CryptoHelper struct {
	srcMode, dstMode crypto.Mode
	srcAead, dstAead cipher.AEAD
}

func NewCryptoHelper(config Config) (*CryptoHelper, error) {
	srcMode, err := crypto.ToMode(config.InDecryptMode)
	if err != nil {
		return nil, err
	}
	dstMode, err := crypto.ToMode(config.OutCryptMode)
	if err != nil {
		return nil, err
	}

	srcAead, err := util.CreateAesGcmAead(util.StringToAesKey(config.InDecryptKey, 32))
	if err != nil {
		return nil, err
	}

	dstAead, err := util.CreateAesGcmAead(util.StringToAesKey(config.OutCryptKey, 32))
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

func (c *CryptoHelper) Copy(dst, src Stream) {
	err := c.copy(dst, src)
	if err != nil {
		if err != io.EOF && err != ErrTimeout {
			logrus.WithError(err).Error("copy")
		}
	}
}

func (c *CryptoHelper) copy(dst, src Stream) error {
	s, err := crypto.NewReaderWriter(src, c.srcMode, c.srcAead)
	if err != nil {
		return err
	}
	d, err := crypto.NewReaderWriter(dst, c.dstMode, c.dstAead)
	if err != nil {
		return err
	}

	if h, ok := src.(CustomCopy); ok {
		in := &CryptoStream{
			rw:     s,
			Stream: src,
		}

		out := &CryptoStream{
			rw:     d,
			Stream: dst,
		}

		err := h.CustomCopy(in, out)
		if err != nil {
			return err
		}
		return nil
	}

	go util.Copy(s, d)
	return util.Copy(d, s)
}

func (c *CryptoHelper) SrcReaderWriter(rw io.ReadWriter) (io.ReadWriter, error) {
	return crypto.NewReaderWriter(rw, c.srcMode, c.srcAead)
}

func (c *CryptoHelper) DstReaderWriter(rw io.ReadWriter) (io.ReadWriter, error) {
	return crypto.NewReaderWriter(rw, c.dstMode, c.dstAead)
}

type CryptoStream struct {
	rw io.ReadWriter
	Stream
}

func (p *CryptoStream) SetReadDeadline(t time.Time) error {
	return p.Stream.SetReadDeadline(t)
}

func (p *CryptoStream) Close() error {
	return p.Stream.Close()
}

func (p *CryptoStream) Read(b []byte) (int, error) {
	return p.rw.Read(b)
}

func (p *CryptoStream) Write(b []byte) (int, error) {
	return p.rw.Write(b)
}

func (c *CryptoHelper) SrcCrypto(s Stream) (Stream, error) {
	rw, err := crypto.NewReaderWriter(s, c.srcMode, c.srcAead)
	if err != nil {
		return nil, err
	}

	return &CryptoStream{
		rw:     rw,
		Stream: s,
	}, nil
}

func (c *CryptoHelper) DstCrypto(s Stream) (Stream, error) {
	rw, err := crypto.NewReaderWriter(s, c.dstMode, c.dstAead)
	if err != nil {
		return nil, err
	}

	return &CryptoStream{
		rw:     rw,
		Stream: s,
	}, nil
}
