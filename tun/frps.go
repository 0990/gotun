package tun

import (
	"fmt"
	"github.com/0990/gotun/tun/msg"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
	"io"
	"time"
)

type Frps struct {
	cfg          Config
	input        input
	worker       input
	cryptoHelper *CryptoHelper

	workerStreams chan Stream
}

func NewFrps(cfg Config) (*Frps, error) {
	input, err := newInput(cfg.Input, cfg.InProtoCfg)
	if err != nil {
		return nil, err
	}

	worker, err := newInput(cfg.Output, cfg.OutProtoCfg)
	if err != nil {
		return nil, err
	}

	c, err := NewCryptoHelper(cfg)
	if err != nil {
		return nil, err
	}

	return &Frps{
		cfg:           cfg,
		input:         input,
		worker:        worker,
		cryptoHelper:  c,
		workerStreams: make(chan Stream, 1000),
	}, nil
}

func (s *Frps) Run() error {
	err := s.input.Run()
	if err != nil {
		return err
	}
	s.input.SetOnNewStream(s.handleInputStream)

	err = s.worker.Run()
	if err != nil {
		return err
	}
	s.worker.SetOnNewStream(s.handlerWorkerStream)
	return nil
}

func (s *Frps) handlerWorkerStream(stream Stream) {
	s.workerStreams <- stream
}

func (s *Frps) getWorkerStream() (Stream, error) {
	select {
	case stream := <-s.workerStreams:
		return stream, nil
	default:
		return nil, fmt.Errorf("no worker stream")
	}
}

func (s *Frps) leftStreamCount() int {
	return len(s.workerStreams)
}

func (s *Frps) Close() error {
	s.input.Close()
	s.worker.Close()
	return nil
}

func (s *Frps) handleInputStream(src Stream) {
	defer src.Close()

	dst, err := s.getWorkerStream()
	if err != nil {
		logrus.WithError(err).Error("openStream")
		return
	}
	defer dst.Close()

	err = s.readHello(dst)
	if err != nil {
		logrus.WithError(err).Error("readHello")
		return
	}

	//增加头标识，用于通知对端此连接开始工作了
	err = s.writeHead(dst)
	if err != nil {
		logrus.WithError(err).Error("writeHead")
		return
	}

	logrus.Debug("stream opened", "in:", src.RemoteAddr(), "out:", fmt.Sprint(dst.RemoteAddr(), "(", dst.ID(), ")"))
	defer logrus.Debug("stream closed", "in:", src.RemoteAddr(), "out:", fmt.Sprint(dst.RemoteAddr(), "(", dst.ID(), ")"))

	err = s.cryptoHelper.Copy(dst, src)
	if err != nil {
		if err != io.EOF {
			logrus.WithError(err).Error("copy")
		}
	}
}

func (s *Frps) readHello(src Stream) error {
	buf := make([]byte, len(FrpcHello))
	src.SetReadDeadline(time.Now().Add(time.Second * 10))
	_, err := io.ReadFull(src, buf)
	if err != nil {
		return err
	}
	if string(buf) != string(FrpcHello) {
		return fmt.Errorf("invalid hello")
	}
	return nil
}

func (s *Frps) writeHead(src Stream) error {
	count := s.leftStreamCount()

	var askWorkerCnt int32
	if count < FRP_WORKER_COUNT {
		askWorkerCnt = FRP_WORKER_COUNT
	}

	msg := msg.FRPHead{
		AskWorkerCnt:  askWorkerCnt,
		LeftWorkerCnt: int32(count),
	}
	data, err := proto.Marshal(&msg)
	if err != nil {
		return err
	}
	nr := len(data)
	buf := make([]byte, 2, nr+2)
	buf[0], buf[1] = byte(nr>>8), byte(nr)
	buf = append(buf, data...)
	_, err = src.Write(buf)
	if err != nil {
		return err
	}
	return nil
}
