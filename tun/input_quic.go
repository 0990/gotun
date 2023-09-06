package tun

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"github.com/quic-go/quic-go"
	"github.com/sirupsen/logrus"
	"math/big"
	"net"
	"sync/atomic"
	"time"
)

type inputQUIC struct {
	inputBase

	addr     string
	cfg      QUICConfig
	listener *quic.Listener

	close int32
}

func NewInputQUIC(addr string, extra string) (*inputQUIC, error) {
	var cfg QUICConfig

	if extra != "" {
		err := json.Unmarshal([]byte(extra), &cfg)
		if err != nil {
			return nil, err
		}
	}

	return &inputQUIC{
		addr: addr,
		cfg:  cfg,
	}, nil
}

func (p *inputQUIC) Run() error {
	lis, err := quic.ListenAddr(p.addr, generateTLSConfig(), nil)
	if err != nil {
		return err
	}
	p.listener = lis
	go p.serve()
	return nil
}

func (p *inputQUIC) serve() {
	var tempDelay time.Duration
	for {
		sess, err := p.listener.Accept(context.Background())
		if err != nil {
			logrus.WithError(err).Error("quicServer Accept")
			if ne, ok := err.(*net.OpError); ok && ne.Temporary() {
				if tempDelay == 0 {
					tempDelay = 5 * time.Millisecond
				} else {
					tempDelay *= 2
				}
				if max := 1 * time.Second; tempDelay > max {
					tempDelay = max
				}
				logrus.Errorf("http: Accept error: %v; retrying in %v", err, tempDelay)
				time.Sleep(tempDelay)
				continue
			}
			return
		}
		go p.handleSession(sess)
	}
}

func (p *inputQUIC) handleSession(session quic.Connection) {
	defer session.CloseWithError(1, "quic server close session")

	for {
		stream, err := session.AcceptStream(context.Background())
		if err != nil {
			return
		}

		if atomic.LoadInt32(&p.close) == 1 {
			return
		}

		s := &QUICStream{
			Stream:     stream,
			localAddr:  session.LocalAddr(),
			remoteAddr: session.RemoteAddr(),
		}
		go func(p1 Stream) {
			p.inputBase.OnNewStream(p1)
		}(s)
	}
}

func (p *inputQUIC) Close() error {
	atomic.StoreInt32(&p.close, 1)
	return p.listener.Close()
}

func generateTLSConfig() *tls.Config {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		panic(err)
	}
	template := x509.Certificate{SerialNumber: big.NewInt(1)}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:    "CERTIFICATE",
		Headers: nil,
		Bytes:   certDER,
	})
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		panic(err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{"quic-stunnel"},
	}
}
