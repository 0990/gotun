package tun

import (
	"fmt"
	"github.com/0990/gotun/core"
	"github.com/0990/gotun/pkg/msg"
	"github.com/sirupsen/logrus"
	"io"
	"sync"
	"time"
)

type frpsControllerManager struct {
	ctl   *frpsController
	mutex sync.Mutex
}

func newFrpsControllerManager() *frpsControllerManager {
	return &frpsControllerManager{}
}

func (f *frpsControllerManager) Get() (*frpsController, bool) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if f.ctl == nil {
		return nil, false
	}
	return f.ctl, true
}

func (f *frpsControllerManager) Set(ctl *frpsController) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	f.ctl = ctl
}

func (f *frpsControllerManager) Close() {
	if f.ctl != nil {
		f.ctl.Close()
	}
}

type frpsController struct {
	ctl     io.ReadWriteCloser
	sendCh  chan (msg.Message)
	readCh  chan (msg.Message)
	workers chan core.IStream
}

func NewFrpsController(ctrl io.ReadWriteCloser) *frpsController {
	return &frpsController{ctl: ctrl, workers: make(chan core.IStream, 100), sendCh: make(chan msg.Message, 100), readCh: make(chan msg.Message, 100)}
}

func (f *frpsController) Run() {
	go f.doWriteLoop()
	//f.Write(&msg.ReqWorkConn{Count: FrpWorkerCount})
	go f.doReadLoop()
	go f.handleReadMsg()
}

func (f *frpsController) Close() {
	f.ctl.Close()
	close(f.workers)
	close(f.sendCh)
	close(f.readCh)
}

func (f *frpsController) RegisterWorker(worker core.IStream) error {
	select {
	case f.workers <- worker:
		logrus.Debug("register worker")
		return nil
	default:
		return fmt.Errorf("worker channel is full")
	}
}

func (f *frpsController) GetWorkConn() (worker core.IStream, err error) {
	if len(f.workers) < FrpWorkerCount/2 {
		err := f.Write(&msg.ReqWorkConn{Count: FrpWorkerCount})
		if err != nil {
			return nil, err
		}
	}

	select {
	case w, ok := <-f.workers:
		if !ok {
			return nil, fmt.Errorf("worker channel is closed")
		}
		return w, nil

	case <-time.After(time.Second * 5):
		return nil, fmt.Errorf("get worker timeout(5s)")
	}
}

func (f *frpsController) doWriteLoop() {
	for {
		select {
		case m, ok := <-f.sendCh:
			if !ok {
				return
			}
			err := msg.WriteMsg(f.ctl, m)
			if err != nil {
				logrus.Errorf("frps controller write msg err:%s", err.Error())
				return
			}
		}
	}
}

func (f *frpsController) Write(msg msg.Message) error {
	select {
	case f.sendCh <- msg:
		return nil
	default:
		return fmt.Errorf("send channel is full")
	}
}

func (f *frpsController) doReadLoop() {
	defer f.ctl.Close()

	for {
		m, err := msg.ReadMsg(f.ctl)
		if err != nil {
			logrus.Errorf("frps controller read msg err:%s", err.Error())
			return
		}
		f.readCh <- m
	}
}

func (f *frpsController) handleReadMsg() {
	for {
		select {
		case m, ok := <-f.readCh:
			if !ok {
				return
			}
			switch m.(type) {
			case *msg.Ping:
				f.Write(&msg.Pong{})
			}
		}
	}
}
