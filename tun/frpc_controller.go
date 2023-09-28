package tun

import (
	"errors"
	"fmt"
	"github.com/0990/gotun/core"
	"github.com/0990/gotun/pkg/msg"
	"github.com/sirupsen/logrus"
	"io"
	"sync/atomic"
	"time"
)

type frpcController struct {
	createConn         func() (core.IStream, error)
	rw                 io.ReadWriteCloser
	cryptoReaderWriter func(rw core.IStream) (core.IStream, error)
	onReqWorkerConn    func(count int32) error

	exit int32
}

func newFrpcController(createConn func() (core.IStream, error), onReqWorkerConn func(count int32) error, cryptoReaderWriter func(rw core.IStream) (core.IStream, error)) *frpcController {
	return &frpcController{
		createConn:         createConn,
		onReqWorkerConn:    onReqWorkerConn,
		cryptoReaderWriter: cryptoReaderWriter,
	}
}

func (c *frpcController) Run(setStatus func(status string)) {
	go c.run(setStatus)
	return
}

func (c *frpcController) run(setStatus func(status string)) {
	for {
		if atomic.LoadInt32(&c.exit) == 1 {
			return
		}

		conn, err := c.createConn()
		if err != nil {
			time.Sleep(time.Second * 5)
			logrus.WithError(err).Error("frpc controller login failed")
			setStatus(fmt.Sprintf("frpc controller login failed:%s", err.Error()))
			continue
		}

		rw, err := c.login(conn)
		if err != nil {
			time.Sleep(time.Second * 5)
			logrus.WithError(err).Error("frpc controller login failed")
			setStatus(fmt.Sprintf("frpc controller login failed:%s", err.Error()))
			continue
		}
		logrus.Info("frpc controller login success")
		c.rw = rw
		go c.keepHeartbeat(rw)
		setStatus("running")
		err = c.onRead(rw)
		if err != nil {
			time.Sleep(time.Second * 5)
			logrus.WithError(err).Error("frpc controller onRead failed")
		}
	}

	return
}

func (c *frpcController) Close() error {
	atomic.StoreInt32(&c.exit, 1)
	if c.rw != nil {
		c.rw.Close()
	}
	return nil
}

func (c *frpcController) login(conn core.IStream) (io.ReadWriteCloser, error) {
	rw, err := c.cryptoReaderWriter(conn)
	if err != nil {
		return nil, err
	}

	err = msg.WriteMsg(rw, &msg.Login{
		Version: "0.0.1",
	})

	if err != nil {
		return nil, err
	}

	conn.SetReadDeadline(time.Now().Add(time.Second * 5))

	m, err := msg.ReadMsg(rw)
	if err != nil {
		return nil, err
	}

	conn.SetReadDeadline(time.Time{})

	resp, ok := m.(*msg.LoginResp)
	if !ok {
		return nil, errors.New("login resp error")
	}

	if resp.Error != "" {
		return nil, errors.New(resp.Error)
	}
	return rw, nil
}

func (c *frpcController) onRead(rw io.ReadWriteCloser) error {
	defer rw.Close()

	for {
		rawMsg, err := msg.ReadMsg(rw)
		if err != nil {
			return err
		}

		switch m := rawMsg.(type) {
		case *msg.ReqWorkConn:
			c.onReqWorkerConn(m.Count)
		case *msg.Pong:
			logrus.Debug("receive heartbeat from server")
		default:

		}
	}
}

func (c *frpcController) keepHeartbeat(rw io.ReadWriteCloser) error {
	ticker := time.NewTicker(time.Second * 30)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m := &msg.Ping{}
			if err := msg.WriteMsg(rw, m); err != nil {
				return err
			}
		}
	}
}
