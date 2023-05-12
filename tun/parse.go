package tun

import (
	"errors"
	"strings"
)

// 解析 0.0.0.0:80/tcp
func parseProtocol(s string) (protocol, string, error) {
	ss := strings.Split(s, "/")
	if len(ss) != 2 {
		return 0, "", errors.New("invalid listen protocol")
	}

	protocol := ss[0]
	proto, err := toProtocol(protocol)
	if err != nil {
		return 0, "", nil
	}

	addr := ss[1]

	return proto, addr, err
}
