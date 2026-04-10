package tun

import (
	"errors"
	"io"
)

type streamRole byte

const (
	streamRoleBusiness  streamRole = 0x00
	streamRoleProbe     streamRole = 0x01
	streamRoleBandwidth streamRole = 0x02
)

var streamHandshakeMagic = [4]byte{'G', 'T', 'H', '1'}

func writeStreamHandshake(w io.Writer, role streamRole) error {
	buf := []byte{
		streamHandshakeMagic[0],
		streamHandshakeMagic[1],
		streamHandshakeMagic[2],
		streamHandshakeMagic[3],
		0x01,
		byte(role),
	}
	_, err := w.Write(buf)
	return err
}

func readStreamHandshake(r io.Reader) (streamRole, error) {
	buf := make([]byte, 6)
	if _, err := io.ReadFull(r, buf); err != nil {
		return streamRoleBusiness, err
	}
	if buf[0] != streamHandshakeMagic[0] ||
		buf[1] != streamHandshakeMagic[1] ||
		buf[2] != streamHandshakeMagic[2] ||
		buf[3] != streamHandshakeMagic[3] {
		return streamRoleBusiness, errors.New("invalid stream handshake magic")
	}
	if buf[4] != 0x01 {
		return streamRoleBusiness, errors.New("unsupported stream handshake version")
	}
	switch streamRole(buf[5]) {
	case streamRoleBusiness, streamRoleProbe, streamRoleBandwidth:
		return streamRole(buf[5]), nil
	default:
		return streamRoleBusiness, errors.New("invalid stream handshake role")
	}
}
