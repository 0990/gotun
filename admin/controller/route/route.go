// Package route 提供智能路由入口的管理 API handler。
// 两种模式（output 出口 / input 入口）共用一套 handler，通过 mode 区分持久化与列表过滤。
package route

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"

	"github.com/0990/gotun/admin/model"
	"github.com/0990/gotun/admin/response"
	"github.com/0990/gotun/tun"
)

// handler 内部复用的处理逻辑，mode 为 tun.RouteModeOutput / tun.RouteModeInput
type handler struct {
	mgr  *tun.Manager
	mode string
}

// ---- DTO 转换 ----

func routeToModel(rs tun.RouteService) model.Route {
	cfg := rs.RouteCfg()

	status := rs.Status()
	if cfg.Disabled {
		status = "disabled"
	}

	health := rs.MemberHealth()
	noHealth := []string{}
	for name, h := range health {
		// 无健康数据（未开帧头探测 → disabled，或非 Server 类型）供 UI 提示
		if h == tun.QualityStatusDisabled {
			noHealth = append(noHealth, name)
		}
	}

	return model.Route{
		UUID:            cfg.UUID,
		Name:            cfg.Name,
		Disabled:        cfg.Disabled,
		Mode:            cfg.Mode,
		Listen:          cfg.Listen,
		Members:         cfg.Members,
		Policy:          cfg.Policy,
		Status:          status,
		PreferredMember: rs.PreferredMember(),
		MemberHealth:    health,
		QualitySummary:  model.QualitySummary(rs.QualitySummary()),
		CreatedAt:       cfg.CreatedAt.Format("2006-01-02 15:04:05"),
		MemberNoHealth:  noHealth,
	}
}

func modelToRoute(m *model.Route) tun.RouteConfig {
	return tun.RouteConfig{
		UUID:     m.UUID,
		Name:     m.Name,
		Disabled: m.Disabled,
		Mode:     m.Mode,
		Listen:   m.Listen,
		Members:  m.Members,
		Policy:   m.Policy,
	}
}

// ---- handlers ----

// List 返回该模式下的全部入口（含实时状态）
func List(mgr *tun.Manager, mode string, version string) func(http.ResponseWriter, *http.Request) {
	h := &handler{mgr: mgr, mode: mode}
	return func(writer http.ResponseWriter, request *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				msg, _ := json.Marshal(response.Ret{Code: http.StatusInternalServerError, Msg: fmt.Sprintf("%v", err)})
				writer.Write(msg)
			}
		}()

		if err := request.ParseForm(); err != nil {
			panic(err.Error())
		}
		page := request.FormValue("page")
		if page == "" {
			page = "1"
		}
		pageInt, err := strconv.Atoi(page)
		if err != nil {
			panic(err.Error())
		}

		routes := h.mgr.AllRoute(h.mode)
		records := []model.Route{}
		for _, rs := range routes {
			records = append(records, routeToModel(rs))
		}
		sort.Slice(records, func(i, j int) bool { return records[i].CreatedAt > records[j].CreatedAt })

		totalNums := len(records)
		pageSize := 20
		totalPages := math.Ceil(float64(totalNums) / float64(pageSize))

		ret := response.Ret{
			Code: http.StatusOK,
			Data: response.List{
				List:    &records,
				Version: version,
				Pagination: response.Pagination{
					PageSize:    pageSize,
					TotalNums:   totalNums,
					TotalPages:  int(totalPages),
					CurrentPage: pageInt,
				},
			},
		}
		d, err := json.Marshal(&ret)
		if err != nil {
			panic(err.Error())
		}
		if _, err := writer.Write(d); err != nil {
			panic(err.Error())
		}
	}
}

// SwitchLog 返回指定入口的最近 member 切换记录（最新在前）
func SwitchLog(mgr *tun.Manager, mode string) func(http.ResponseWriter, *http.Request) {
	h := &handler{mgr: mgr, mode: mode}
	return func(writer http.ResponseWriter, request *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				msg, _ := json.Marshal(response.Ret{Code: http.StatusInternalServerError, Msg: fmt.Sprintf("%v", err)})
				writer.Write(msg)
			}
		}()

		if err := request.ParseForm(); err != nil {
			panic(err.Error())
		}
		name := request.FormValue("name")
		if name == "" {
			panic("lose name")
		}

		svc, ok := h.mgr.GetRoute(name)
		if !ok || svc.RouteCfg().Mode != h.mode {
			panic("route not exist")
		}

		records := []model.RouteSwitchEvent{}
		for _, e := range svc.SwitchEvents() {
			records = append(records, model.RouteSwitchEvent{
				Time: e.Time.Format("2006-01-02 15:04:05"),
				From: e.From,
				To:   e.To,
			})
		}

		d, err := json.Marshal(&response.Ret{Code: http.StatusOK, Data: records})
		if err != nil {
			panic(err.Error())
		}
		if _, err := writer.Write(d); err != nil {
			panic(err.Error())
		}
	}
}

// Create 新建入口（禁用态可直接创建；启用态创建失败会返回校验错误）
func Create(mgr *tun.Manager, mode string) func(http.ResponseWriter, *http.Request) {
	h := &handler{mgr: mgr, mode: mode}
	return func(writer http.ResponseWriter, request *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				msg, _ := json.Marshal(response.Ret{Code: http.StatusInternalServerError, Msg: fmt.Sprintf("%v", err)})
				writer.Write(msg)
			}
		}()

		body, err := io.ReadAll(request.Body)
		if err != nil {
			panic(err.Error())
		}
		data := model.Route{}
		if err := json.Unmarshal(body, &data); err != nil {
			panic(err.Error())
		}
		data.Mode = h.mode // 模式由端点决定，不信任客户端

		cfg := modelToRoute(&data)
		if err := h.mgr.AddRoute(cfg, true); err != nil {
			panic(err.Error())
		}

		d, _ := json.Marshal(&response.Ret{Code: http.StatusOK, Msg: "success"})
		if _, err := writer.Write(d); err != nil {
			panic(err.Error())
		}
	}
}

// Edit 按 UUID 编辑入口
func Edit(mgr *tun.Manager, mode string) func(http.ResponseWriter, *http.Request) {
	h := &handler{mgr: mgr, mode: mode}
	return func(writer http.ResponseWriter, request *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				msg, _ := json.Marshal(response.Ret{Code: http.StatusInternalServerError, Msg: fmt.Sprintf("%v", err)})
				writer.Write(msg)
			}
		}()

		body, err := io.ReadAll(request.Body)
		if err != nil {
			panic(err.Error())
		}
		data := model.Route{}
		if err := json.Unmarshal(body, &data); err != nil {
			panic(err.Error())
		}
		data.Mode = h.mode

		cfg := modelToRoute(&data)
		if err := h.mgr.ReplaceRouteByUUID(cfg); err != nil {
			panic(err.Error())
		}

		d, _ := json.Marshal(&response.Ret{Code: http.StatusOK, Msg: "success"})
		if _, err := writer.Write(d); err != nil {
			panic(err.Error())
		}
	}
}

// Delete 按 UUID 删除入口
func Delete(mgr *tun.Manager, mode string) func(http.ResponseWriter, *http.Request) {
	h := &handler{mgr: mgr, mode: mode}
	return func(writer http.ResponseWriter, request *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				msg, _ := json.Marshal(response.Ret{Code: http.StatusInternalServerError, Msg: fmt.Sprintf("%v", err)})
				writer.Write(msg)
			}
		}()

		body, err := io.ReadAll(request.Body)
		if err != nil {
			panic(err.Error())
		}
		data := model.Route{}
		if err := json.Unmarshal(body, &data); err != nil {
			panic(err.Error())
		}

		if err := h.mgr.RemoveRouteByUUID(data.UUID); err != nil {
			panic(err.Error())
		}

		d, _ := json.Marshal(&response.Ret{Code: http.StatusOK, Msg: "success"})
		if _, err := writer.Write(d); err != nil {
			panic(err.Error())
		}
	}
}

// SetDisabled 启用/禁用入口
func SetDisabled(mgr *tun.Manager, mode string) func(http.ResponseWriter, *http.Request) {
	h := &handler{mgr: mgr, mode: mode}
	return func(writer http.ResponseWriter, request *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				msg, _ := json.Marshal(response.Ret{Code: http.StatusInternalServerError, Msg: fmt.Sprintf("%v", err)})
				writer.Write(msg)
			}
		}()

		if err := request.ParseForm(); err != nil {
			panic(err.Error())
		}
		name := request.FormValue("name")
		disabledStr := request.FormValue("disabled")
		if name == "" {
			panic("lose name")
		}
		disabled, err := strconv.ParseBool(disabledStr)
		if err != nil {
			panic(err.Error())
		}

		if err := h.mgr.SetRouteDisabled(name, disabled); err != nil {
			panic(err.Error())
		}

		d, _ := json.Marshal(&response.Ret{Code: http.StatusOK, Msg: "success"})
		if _, err := writer.Write(d); err != nil {
			panic(err.Error())
		}
	}
}
