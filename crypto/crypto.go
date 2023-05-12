package crypto

import (
	"crypto/cipher"
	"errors"
	"io"
	"math/rand"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

func NewReaderWriter(rw io.ReadWriter, mode Mode, aead cipher.AEAD) (io.ReadWriter, error) {
	switch mode {
	case None:
		return rw, nil
	case GCM:
		return newGCM(rw, aead), nil
	default:
		return nil, errors.New("unknown mode")
	}
}
