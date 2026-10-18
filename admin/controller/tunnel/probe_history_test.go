package tunnel

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0990/gotun/pkg/probehistory"
	"github.com/0990/gotun/tun"
)

func TestProbeHistoryHandler(t *testing.T) {
	dir := t.TempDir()
	mgr := tun.NewManager(dir)
	if err := mgr.OpenProbeHistory(); err != nil {
		t.Fatal(err)
	}
	defer mgr.CloseProbeHistory()
	if err := mgr.AddService(tun.Config{UUID: "a", Name: "已停用隧道", Disabled: true}, false); err != nil {
		t.Fatal(err)
	}
	// 用第二个连接写入，模拟服务重启前已存在于磁盘的历史。
	w, err := probehistory.Open(filepath.Join(dir, "probe-history.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	for _, uuid := range []string{"a", "b"} {
		if err := w.Record(probehistory.Record{TunnelUUID: uuid, Time: time.Now().Add(-time.Minute), Kind: "route", Status: "failed", RouteName: "入口", Error: "超时"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		query string
		code  int
		count int
	}{
		{"uuid=a&kind=route&status=failed", 200, 1},
		{"uuid=a&kind=frame", 200, 0},
		{"uuid=a&status=normal", 200, 0},
		{"uuid=a&since=2000&until=1000", 400, 0},
		{"uuid=a&before=-1", 400, 0},
		{"uuid=a&since=abc", 400, 0},
		{"uuid=a&kind=unknown", 400, 0},
		{"uuid=a&status=unknown", 400, 0},
		{"uuid=unknown", 404, 0},
		{"", 400, 0},
	} {
		t.Run(tc.query, func(t *testing.T) {
			response := httptest.NewRecorder()
			ProbeHistory(mgr)(response, httptest.NewRequest("GET", "/api/tunnel/probe_history?"+tc.query, nil))
			var ret struct {
				Code int               `json:"code"`
				Data probehistory.Page `json:"data"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &ret); err != nil {
				t.Fatal(err)
			}
			if ret.Code != tc.code || len(ret.Data.Records) != tc.count {
				t.Fatalf("错误响应：%s", response.Body.String())
			}
			if tc.count > 0 && (!strings.Contains(response.Body.String(), "+08:00") || ret.Data.Records[0].TunnelUUID != "a") {
				t.Fatal("应返回北京时间且不能串隧道")
			}
		})
	}
}
