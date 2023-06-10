package crypto

import (
	"crypto/cipher"
	"io"
	"math/rand"
)

// TODO 设置为多少合适，或者writer使用动态buff?
// 为什么是65*1024，当为64*1024时,由于上层copybuff使用了64*1024的缓冲区，这里加密后长度会大于64*1024，导致copybuff出错
const payloadSizeMask = 65 * 1024

type gcm struct {
	io.ReadWriter
	r *reader
	w *writer
}

// 读操作：从rw读数据且解密，返回解密后的数据；写操作：加密后写入rw，
func newGCM(rw io.ReadWriter, aead cipher.AEAD) io.ReadWriter {
	if aead == nil {
		return rw
	}
	return &gcm{
		ReadWriter: rw,
		r:          NewReader(rw, aead),
		w:          NewWriter(rw, aead),
	}
}

func (p *gcm) Read(b []byte) (int, error) {
	return p.r.Read(b)
}

func (c *gcm) Write(b []byte) (int, error) {
	return c.w.Write(b)
}

type writer struct {
	io.Writer
	cipher.AEAD
	nonce []byte
	buf   []byte
}

func NewWriter(w io.Writer, aead cipher.AEAD) *writer {
	return &writer{
		Writer: w,
		AEAD:   aead,
		buf:    make([]byte, 2+aead.Overhead()+payloadSizeMask+aead.Overhead()),
		nonce:  make([]byte, aead.NonceSize()),
	}
}

func (w *writer) RandomNonce() {

}

// 加密后写入rw，但返回的是b的长度，而不是加密后的长度（为了兼容io.Writer接口）
func (w *writer) Write(b []byte) (int, error) {
	n, err := w.write(b)
	return int(n), err
}

// nonce + payloadsize(encrypt)+ payload(encrypt)
func (w *writer) write(b []byte) (n int64, err error) {
	if len(b) == 0 {
		return 0, nil
	}

	//create new nonce
	rand.Read(w.nonce)

	buf := w.buf
	sizeBuf := buf[w.NonceSize():]
	payloadBuf := buf[2+w.Overhead()+w.NonceSize() : 2+w.Overhead()+w.NonceSize()+payloadSizeMask]
	nr := len(b)

	n += int64(nr)
	end := w.NonceSize() + 2 + w.Overhead() + nr + w.Overhead()
	buf = buf[:end]
	sizeBuf[0], sizeBuf[1] = byte(nr>>8), byte(nr)

	//nonce
	copy(buf, w.nonce)
	//payloadsize
	w.Seal(sizeBuf[:0], w.nonce, sizeBuf[:2], nil)
	//payload
	w.Seal(payloadBuf[:0], w.nonce, b, nil)
	_, ew := w.Writer.Write(buf)
	if ew != nil {
		return n, ew
	}

	return n, nil
}

type reader struct {
	io.Reader
	cipher.AEAD
	nonce    []byte
	buf      []byte
	leftover []byte
}

func NewReader(r io.Reader, aead cipher.AEAD) *reader {
	return &reader{
		Reader: r,
		AEAD:   aead,
		nonce:  make([]byte, aead.NonceSize()),
		buf:    make([]byte, payloadSizeMask+aead.Overhead()),
	}
}

// 解密后copy到b中，返回的是解密后的数据长度，而不是解密前的长度（为了兼容io.Reader接口）
func (r *reader) Read(b []byte) (int, error) {
	if len(r.leftover) > 0 {
		n := copy(b, r.leftover)
		r.leftover = r.leftover[n:]
		return n, nil
	}

	n, err := r.read()
	m := copy(b, r.buf[:n])
	if m < n {
		r.leftover = r.buf[m:n]
	}
	return m, err
}

func (r *reader) read() (int, error) {
	_, err := io.ReadFull(r.Reader, r.nonce)
	if err != nil {
		return 0, err
	}
	buf := r.buf[:2+r.Overhead()]
	_, err = io.ReadFull(r.Reader, buf)
	if err != nil {
		return 0, err
	}
	_, err = r.Open(buf[:0], r.nonce, buf, nil)
	if err != nil {
		return 0, err
	}

	size := (int(buf[0])<<8 + int(buf[1])) & payloadSizeMask
	buf = r.buf[:size+r.Overhead()]
	_, err = io.ReadFull(r.Reader, buf)
	if err != nil {
		return 0, err
	}
	_, err = r.Open(buf[:0], r.nonce, buf, nil)
	if err != nil {
		return 0, err
	}
	return size, nil
}
