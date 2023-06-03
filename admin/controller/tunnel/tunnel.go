package tunnel

import (
	"encoding/json"
	"fmt"
	"github.com/0990/gotun/tun"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/0990/gotun/admin/model"
	"github.com/0990/gotun/admin/response"
)

func List(mgr *tun.Manager) func(writer http.ResponseWriter, request *http.Request) {
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

		// Get page
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

		var cfgs []tun.Config
		for _, v := range ss {
			cfgs = append(cfgs, v.Cfg())
		}

		sort.Slice(cfgs, func(i, j int) bool {
			return cfgs[i].CreatedAt.Unix() > cfgs[j].CreatedAt.Unix()
		})

		var records []model.Tunnel
		for _, cfg := range cfgs {
			records = append(records, Config2Model(cfg))
		}

		if records == nil {
			records = make([]model.Tunnel, 0)
		}

		totalNums := len(records)
		pageSize := 20
		totalPages := math.Ceil(float64(totalNums) / float64(pageSize))

		ret := response.Ret{
			Code: http.StatusOK,
			Data: response.List{
				List: &records,
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

func Create(mgr *tun.Manager) func(writer http.ResponseWriter, request *http.Request) {
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

		data := model.Tunnel{}
		err = json.Unmarshal(body, &data)
		if err != nil {
			panic(err.Error())
		}

		cfg, err := Model2Config(&data)
		if err != nil {
			panic(err.Error())
		}

		now := time.Now()
		cfg.CreatedAt = now

		err = mgr.AddService(*cfg, true)
		if err != nil {
			panic(err.Error())
		}

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
}

func Model2Config(ls *model.Tunnel) (*tun.Config, error) {
	return &tun.Config{
		Name:          ls.Name,
		Input:         ls.Input,
		Output:        ls.Output,
		Mode:          ls.Mode,
		InProtoCfg:    ls.InProtoCfg,
		InDecryptMode: ls.InDecryptMode,
		InDecryptKey:  ls.InDecryptKey,
		InExtend:      ls.InExtend,
		OutProtoCfg:   ls.OutProtoCfg,
		OutCryptMode:  ls.OutCryptMode,
		OutCryptKey:   ls.OutCryptKey,
		OutExtend:     ls.OutExtend,
	}, nil
}

func Config2Model(ls tun.Config) model.Tunnel {
	return model.Tunnel{
		Name:          ls.Name,
		Input:         ls.Input,
		Output:        ls.Output,
		Mode:          ls.Mode,
		InProtoCfg:    ls.InProtoCfg,
		InDecryptMode: ls.InDecryptMode,
		InDecryptKey:  ls.InDecryptKey,
		InExtend:      ls.InExtend,
		OutProtoCfg:   ls.OutProtoCfg,
		OutCryptMode:  ls.OutCryptMode,
		OutCryptKey:   ls.OutCryptKey,
		OutExtend:     ls.OutExtend,
		CreatedAt:     ParseTime2String(ls.CreatedAt),
	}
}

func ParseTime2String(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
}

func Edit(mgr *tun.Manager) func(writer http.ResponseWriter, request *http.Request) {
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

		data := model.Tunnel{}
		err = json.Unmarshal(body, &data)
		if err != nil {
			panic(err.Error())
		}

		cfg, err := Model2Config(&data)
		if err != nil {
			panic(err.Error())
		}

		err = mgr.RemoveService(cfg.Name)
		if err != nil {
			panic(err.Error())
		}

		now := time.Now()
		cfg.CreatedAt = now
		err = mgr.AddService(*cfg, true)
		if err != nil {
			panic(err.Error())
		}

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
}

func Delete(mgr *tun.Manager) func(writer http.ResponseWriter, request *http.Request) {
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

		data := model.Tunnel{}
		err = json.Unmarshal(body, &data)
		if err != nil {
			panic(err.Error())
		}

		cfg, err := Model2Config(&data)
		if err != nil {
			panic(err.Error())
		}

		err = mgr.RemoveService(cfg.Name)
		if err != nil {
			panic(err.Error())
		}

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
}
