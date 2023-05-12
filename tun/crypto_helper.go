package tun

import (
	"crypto/cipher"
	"github.com/0990/gotun/crypto"
	"github.com/0990/gotun/util"
	"io"
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

func (c *CryptoHelper) Copy(dst, src io.ReadWriter) error {
	s, err := crypto.NewReaderWriter(src, c.srcMode, c.srcAead)
	if err != nil {
		return err
	}
	d, err := crypto.NewReaderWriter(dst, c.dstMode, c.dstAead)
	if err != nil {
		return err
	}

	go util.Copy(s, d)
	return util.Copy(d, s)
}
