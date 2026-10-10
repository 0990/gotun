package util

import (
	"testing"
	"time"
)

func TestFormatBeijingTime(t *testing.T) {
	instant := time.Date(2026, 10, 9, 18, 6, 13, 0, time.UTC)
	for _, zone := range []*time.Location{time.UTC, time.FixedZone("UTC-7", -7*60*60), BeijingLocation} {
		input := instant.In(zone)
		if got := FormatBeijingTime(input, "2006-01-02 15:04:05"); got != "2026-10-10 02:06:13" {
			t.Fatalf("%s 时间转换错误：%s", zone, got)
		}
		if got := FormatBeijingTime(input, time.RFC3339); got != "2026-10-10T02:06:13+08:00" {
			t.Fatalf("%s 时区标记错误：%s", zone, got)
		}
	}
}
