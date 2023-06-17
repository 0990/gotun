package util

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"io"
	"net"
	"time"
)

const socketBufSize = 64 * 1024

func CreateAesGcmAead(key []byte) (cipher.AEAD, error) {
	if len(key) != 32 {
		return nil, errors.New("key len!=32")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return aead, nil
}

func StringToAesKey(password string, keyLen int) []byte {
	var b, prev []byte
	h := md5.New()
	for len(b) < keyLen {
		h.Write(prev)
		h.Write([]byte(password))
		c := h.Sum(b)
		b = c
		prev = b[len(b)-h.Size():]
		h.Reset()
	}
	return b[:keyLen]
}

func Copy(dst io.Writer, src io.Reader) error {
	_, err := io.Copy(dst, src)
	return err
}

func CopyEach(dst net.Conn, src net.Conn, timeout time.Duration) {
	go func() {
		CopyWithTimeout(dst, src, timeout)
	}()

	CopyWithTimeout(src, dst, timeout)
}

func CopyWithTimeout(dst net.Conn, src net.Conn, timeout time.Duration) error {
	b := make([]byte, socketBufSize)
	for {
		if timeout != 0 {
			src.SetReadDeadline(time.Now().Add(timeout))
		}
		n, err := src.Read(b)
		if err != nil {
			return fmt.Errorf("copy read:%w", err)
		}
		wn, err := dst.Write(b[0:n])
		if err != nil {
			return fmt.Errorf("copy write:%w", err)
		}
		if wn != n {
			return fmt.Errorf("copy write not full")
		}
	}
	return nil
}

func NewUUID() string {
	return uuid.New().String()
}
