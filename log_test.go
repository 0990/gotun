package gotun

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func TestBeijingLogFormatter(t *testing.T) {
	instant := time.Date(2026, 10, 9, 18, 6, 13, 0, time.UTC)
	entry := logrus.NewEntry(logrus.New())
	entry.Time = instant
	entry.Message = "测试日志"
	for _, formatter := range []logrus.Formatter{
		&logrus.TextFormatter{FullTimestamp: true, DisableColors: true},
		&logrus.JSONFormatter{},
	} {
		output, err := (beijingFormatter{Formatter: formatter}).Format(entry)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(output), "2026-10-10T02:06:13+08:00") {
			t.Fatalf("日志时间应为北京时间：%s", output)
		}
		if entry.Time != instant {
			t.Fatal("格式化不应修改原始时间")
		}
		if _, ok := formatter.(*logrus.JSONFormatter); ok && !json.Valid(output) {
			t.Fatalf("JSON 日志格式错误：%s", output)
		}
	}
}
