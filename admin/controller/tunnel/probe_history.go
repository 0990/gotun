package tunnel

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/0990/gotun/admin/response"
	"github.com/0990/gotun/pkg/probehistory"
	"github.com/0990/gotun/pkg/util"
	"github.com/0990/gotun/tun"
)

// ProbeHistory 按隧道 UUID 查询逐次探测，游标分页避免新结果造成翻页重复。
func ProbeHistory(mgr *tun.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		reply := func(code int, message string, data any) {
			_ = json.NewEncoder(w).Encode(response.Ret{Code: code, Msg: message, Data: data})
		}
		uuid := r.URL.Query().Get("uuid")
		if uuid == "" {
			reply(400, "缺少隧道 UUID", nil)
			return
		}
		if _, ok := mgr.GetServiceByUUID(uuid); !ok {
			reply(404, "隧道不存在", nil)
			return
		}
		f := probehistory.Filter{TunnelUUID: uuid, Kind: r.URL.Query().Get("kind"), Status: r.URL.Query().Get("status"), Limit: 50}
		if f.Kind != "" && f.Kind != "frame" && f.Kind != "route" {
			reply(400, "未知探测类型", nil)
			return
		}
		switch f.Status {
		case "", "normal", "slow", "failed", "common_failure":
		default:
			reply(400, "未知探测结果", nil)
			return
		}
		for _, param := range []string{"since", "until", "before"} {
			value := r.URL.Query().Get(param)
			if value == "" {
				continue
			}
			n, err := strconv.ParseInt(value, 10, 64)
			if err != nil || n <= 0 {
				reply(400, "无效的查询参数："+param, nil)
				return
			}
			switch param {
			case "since":
				f.Since = time.UnixMilli(n)
			case "until":
				f.Until = time.UnixMilli(n)
			case "before":
				f.BeforeID = n
			}
		}
		if f.Until.IsZero() {
			f.Until = time.Now()
		}
		if f.Since.After(f.Until) {
			reply(400, "开始时间不能晚于结束时间", nil)
			return
		}
		page, err := mgr.QueryProbeHistory(r.Context(), f)
		if err != nil {
			reply(500, "查询探测历史失败："+err.Error(), nil)
			return
		}
		for i := range page.Records {
			page.Records[i].Time = page.Records[i].Time.In(util.BeijingLocation)
		}
		reply(200, "", page)
	}
}
