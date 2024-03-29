package tun

import (
	"github.com/0990/gotun/core"
	"github.com/0990/gotun/pkg/msg"
	"github.com/0990/socks5"
	"github.com/sirupsen/logrus"
	"net"
)

type Socks5XConn struct {
	net.Conn

	cfg InProtoSocks5X
}

func (c *Socks5XConn) ID() string {
	return "socks5xconn"
}

type CustomCopy interface {
	CustomCopy(in, out core.IStream, id string) error
}

func (c *Socks5XConn) CustomCopy(in, out core.IStream, id string) error {
	s5 := socks5.NewConn(in, socks5.ConnCfg{
		UserName:          c.cfg.UserName,
		Password:          c.cfg.Password,
		TCPTimeout:        c.cfg.TCPTimeout,
		UDPAdvertisedIP:   c.cfg.UDPAdvertisedIP,
		UDPAdvertisedPort: c.cfg.UDPAdvertisedPort,
	})

	s5.SetCustomDialTarget(func(addr string) (socks5.Stream, byte, string, error) {
		logrus.WithFields(logrus.Fields{
			"target": addr,
			"id":     id,
		}).Debug("dial target")

		err := msg.WriteMsg(out, &msg.Socks5XReq{
			TargetAddr: addr,
		})

		if err != nil {
			return nil, 0, "", err
		}

		//TODO msg.Socks5XResp could give more connect information to the client, but add a round trip,comment it for now
		//out.SetReadDeadline(time.Now().Add(time.Second * 5))
		//m, err := msg.ReadMsg(out)
		//if err != nil {
		//	return nil, 0, "", err
		//}
		//out.SetReadDeadline(time.Time{})
		//
		//resp, ok := m.(*msg.Socks5XResp)
		//if !ok {
		//	return nil, 0, "", errors.New("socks5x error:invalid resp")
		//}
		//
		//if err != nil {
		//	return nil, 0, "", err
		//}
		//
		//if resp.Rep != 0 {
		//	return nil, resp.Rep, "", fmt.Errorf("socks5x error:rep:%d", resp.Rep)
		//}

		return out, socks5.RepSuccess, out.LocalAddr().String(), nil
	})

	return s5.Handle()
}
