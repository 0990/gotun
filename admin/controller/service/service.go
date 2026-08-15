package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"

	"github.com/0990/gotun/admin/response"
	"github.com/0990/gotun/server/service"
)

// Record 内置服务实例的对外展示模型（配置 + 运行状态）
type Record struct {
	service.Config
	Status string `json:"status"`
}

// List 内置服务实例列表
func List(mgr *service.Manager, version string) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				msg, _ := json.Marshal(response.Ret{
					Code: http.StatusInternalServerError,
					Msg:  fmt.Sprintf("%v", err),
				})
				writer.Write(msg)
			}
		}()

		err := request.ParseForm()
		if err != nil {
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

		ss := mgr.AllService()
		var records []Record
		for _, v := range ss {
			records = append(records, Record{
				Config: v.Cfg(),
				Status: v.Status(),
			})
		}
		if records == nil {
			records = make([]Record, 0)
		}

		sort.Slice(records, func(i, j int) bool {
			return records[i].CreatedAt.Unix() > records[j].CreatedAt.Unix()
		})

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
		_, err = writer.Write(d)
		if err != nil {
			panic(err.Error())
		}
	}
}

// Create 新建内置服务实例
func Create(mgr *service.Manager) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				msg, _ := json.Marshal(response.Ret{
					Code: http.StatusInternalServerError,
					Msg:  fmt.Sprintf("%v", err),
				})
				writer.Write(msg)
			}
		}()

		cfg := parseBodyConfig(request)
		if cfg.Name == "" {
			panic(errors.New("lose name"))
		}
		if cfg.Type == "" {
			panic(errors.New("lose type"))
		}

		if err := mgr.AddService(cfg, true); err != nil {
			panic(err.Error())
		}

		writeOK(writer)
	}
}

// Edit 编辑内置服务实例（按 UUID 替换）
func Edit(mgr *service.Manager) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				msg, _ := json.Marshal(response.Ret{
					Code: http.StatusInternalServerError,
					Msg:  fmt.Sprintf("%v", err),
				})
				writer.Write(msg)
			}
		}()

		cfg := parseBodyConfig(request)
		if cfg.UUID == "" {
			panic(errors.New("lose uuid"))
		}

		if err := mgr.ReplaceServiceByUUID(cfg); err != nil {
			panic(err.Error())
		}

		writeOK(writer)
	}
}

// Delete 删除内置服务实例
func Delete(mgr *service.Manager) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				msg, _ := json.Marshal(response.Ret{
					Code: http.StatusInternalServerError,
					Msg:  fmt.Sprintf("%v", err),
				})
				writer.Write(msg)
			}
		}()

		body, err := io.ReadAll(request.Body)
		if err != nil {
			panic(err.Error())
		}
		payload := struct {
			Name string `json:"name"`
		}{}
		if err := json.Unmarshal(body, &payload); err != nil {
			panic(err.Error())
		}
		if payload.Name == "" {
			panic(errors.New("lose name"))
		}

		if err := mgr.RemoveService(payload.Name); err != nil {
			panic(err.Error())
		}

		writeOK(writer)
	}
}

// SetDisabled 启用/禁用内置服务实例
func SetDisabled(mgr *service.Manager) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				msg, _ := json.Marshal(response.Ret{
					Code: http.StatusInternalServerError,
					Msg:  fmt.Sprintf("%v", err),
				})
				writer.Write(msg)
			}
		}()

		body, err := io.ReadAll(request.Body)
		if err != nil {
			panic(err.Error())
		}
		payload := struct {
			Name     string `json:"name"`
			Disabled bool   `json:"disabled"`
		}{}
		if err := json.Unmarshal(body, &payload); err != nil {
			panic(err.Error())
		}
		if payload.Name == "" {
			panic(errors.New("lose name"))
		}

		if err := mgr.SetServiceDisabled(payload.Name, payload.Disabled); err != nil {
			panic(err.Error())
		}

		writeOK(writer)
	}
}

// parseBodyConfig 从请求体解析内置服务配置
func parseBodyConfig(request *http.Request) service.Config {
	body, err := io.ReadAll(request.Body)
	if err != nil {
		panic(err.Error())
	}
	cfg := service.Config{}
	if err := json.Unmarshal(body, &cfg); err != nil {
		panic(err.Error())
	}
	return cfg
}

func writeOK(writer http.ResponseWriter) {
	ret := response.Ret{
		Code: http.StatusOK,
		Msg:  "success",
	}
	d, err := json.Marshal(&ret)
	if err != nil {
		panic(err.Error())
	}
	_, err = writer.Write(d)
	if err != nil {
		panic(err.Error())
	}
}
