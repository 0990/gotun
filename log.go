package gotun

import (
	"bytes"
	"errors"
	"fmt"
	rotatelogs "github.com/lestrrat/go-file-rotatelogs"
	"github.com/natefinch/lumberjack"
	"github.com/sirupsen/logrus"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	// ErrCloesd ...
	ErrCloesd = errors.New("ErrCloesd")
)

func InitLog(logpath string, name string, useJSON bool, rotate bool, maxSize int, level logrus.Level) (close func(), err error) {

	//logrus.SetOutput(os.Stdout)
	//if runtime.GOOS == "windows" {
	//	logrus.SetFormatter(&log.TextFormatter{DisableColors: true, FullTimestamp: true})
	//} else {
	//	logrus.SetFormatter(&log.TextFormatter{DisableColors: false, FullTimestamp: true})
	//}
	logrus.SetReportCaller(true)
	logrus.SetLevel(level)
	//log.SetFormatter(new(log.JSONFormatter))
	text := new(logrus.TextFormatter)
	text.FullTimestamp = true
	text.DisableColors = true
	text.CallerPrettyfier = func(frame *runtime.Frame) (function string, file string) {
		//处理文件名
		fileName := path.Base(frame.File)
		fileName += ":" + strconv.Itoa(frame.Line)
		s := strings.Split(frame.Function, ".")
		funcname := s[len(s)-1]
		return funcname + " " + fileName, ""
	}
	logrus.SetFormatter(text)

	hook, close, err := NewFileLogHook(logpath, name, useJSON, rotate, maxSize)
	if err != nil {
		return nil, err
	}

	logrus.AddHook(hook)

	return close, nil
}

// NewFileLogHook 异步记录本地文件日志插件 for logrus
func NewFileLogHook(dir string, filename string, useJSONFormat bool, rotate bool, maxSize int) (hook *Hook, close func(), err error) {
	os.Mkdir(dir, os.ModePerm)

	dir, err = filepath.Abs(dir)
	if err != nil {
		return nil, nil, err
	}
	// Abs 会调用 Clean 方法, 因此会去除dir结尾的“/”
	dir += "/"

	var f io.WriteCloser

	if rotate {
		if maxSize > 0 {
			f = &lumberjack.Logger{
				Filename:   dir + filename + ".log",
				MaxSize:    maxSize,
				MaxBackups: 0,
				MaxAge:     0,
				Compress:   false,
				LocalTime:  true,
			}
		} else {
			f, err = rotatelogs.New(
				dir+filename+".%Y%m%d.log", //%H%M
				//rotatelogs.WithLinkName(dir+filename+".log"),
				rotatelogs.WithMaxAge(15*24*time.Hour),
				rotatelogs.WithRotationTime(24*time.Hour),
				//rotatelogs.WithRotationTime(time.Minute),
			)
			if err != nil {
				return nil, nil, err
			}
		}
	} else {
		f, err = os.OpenFile(dir+filename+".log", os.O_WRONLY|os.O_CREATE|os.O_APPEND, os.ModePerm)
		if err != nil {
			return nil, nil, err
		}
	}

	var newHook *Hook
	if useJSONFormat {
		newHook = NewHook(4096, f, new(logrus.JSONFormatter))
	} else {
		newHook = NewHook(4096, f, new(logrus.TextFormatter))
	}

	close = func() {
		newHook.Close()
		f.Close()
	}

	return newHook, close, nil
}

// Hook io Writer for logrus
type Hook struct {
	w    io.Writer
	fmt  logrus.Formatter
	pool sync.Pool

	toWrite chan *logrus.Entry
	closed  int32
	wg      sync.WaitGroup
}

// New Hook
func NewHook(bufSize int, w io.Writer, fmtt logrus.Formatter) *Hook {
	h := new(Hook)
	h.w = w
	h.fmt = fmtt
	h.pool.New = func() interface{} {
		return new(bytes.Buffer)
	}
	h.toWrite = make(chan *logrus.Entry, bufSize)
	SafeGo(func() {
		for entry := range h.toWrite {
			err := h.WriteEntry(entry)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to async log: %v\n", err)
			}
			h.wg.Done()
		}
	})
	return h
}

// Levels logrus.Hook interface
func (h *Hook) Levels() []logrus.Level {
	return logrus.AllLevels
}

// Fire logrus.Hook interface
func (h *Hook) Fire(entry *logrus.Entry) error {
	if atomic.LoadInt32(&h.closed) != 0 {
		return ErrCloesd
	}

	newEntry := logrus.NewEntry(entry.Logger)
	newEntry.Time = entry.Time
	newEntry.Level = entry.Level
	newEntry.Message = entry.Message
	for k, v := range entry.Data {
		newEntry.Data[k] = v
	}
	h.wg.Add(1)
	h.toWrite <- newEntry
	return nil
}

// Close 关闭异步记录循环, 该调用会等待所有操作完成.
func (h *Hook) Close() {
	atomic.StoreInt32(&h.closed, 1)
	close(h.toWrite)
	h.wg.Wait()
}

// Fire logrus.Hook interface
func (h *Hook) WriteEntry(entry *logrus.Entry) error {
	buffer := h.pool.Get().(*bytes.Buffer)
	buffer.Reset()
	defer h.pool.Put(buffer)
	entry.Buffer = buffer
	serialized, err := h.fmt.Format(entry)
	entry.Buffer = nil
	_, err = h.w.Write(serialized)
	return err
}

func Recover() {
	if e := recover(); e != nil {
		stack := debug.Stack()
		err := fmt.Sprintf("%v\n", e)
		logrus.WithFields(logrus.Fields{
			"err":   err,
			"stack": string(stack),
		}).Error("Recover")

		os.Stderr.Write([]byte(err))
		os.Stderr.Write(stack)

		//fmt.Printf("%v\n", e)
		//fmt.Printf(string(debug.Stack()))
		//_, _ = os.Stderr.Write([]byte(fmt.Sprintf("%v\n%s", e, debug.Stack())))
	}
}

// SafeGo go
func SafeGo(f func()) {
	if f != nil {
		go func() {
			defer Recover()
			f()
		}()
	}
}
