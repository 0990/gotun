package tunnel

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/0990/gotun/server/echo"
	"github.com/0990/gotun/server/httpproxy"
	"github.com/0990/gotun/server/socks5client"
	"github.com/0990/gotun/tun"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/0990/gotun/admin/model"
	"github.com/0990/gotun/admin/response"
)

func List(mgr *tun.GroupManager, version string) func(writer http.ResponseWriter, request *http.Request) {
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

		ss := mgr.AllGroups()

		type ConfigX struct {
			tun.GroupConfig
			Status string
		}

		var cfgs []ConfigX
		for _, v := range ss {
			cfg := v.Cfg()
			status := v.Status()
			cfgs = append(cfgs, ConfigX{
				Config: cfg,
				Status: status,
			})
		}

		sort.Slice(cfgs, func(i, j int) bool {
			return cfgs[i].CreatedAt.Unix() > cfgs[j].CreatedAt.Unix()
		})

		var records []model.Tunnel
		for _, cfg := range cfgs {
			record := Config2Model(cfg.Config)
			record.Status = cfg.Status
			records = append(records, record)
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

func Create(mgr *tun.GroupManager) func(writer http.ResponseWriter, request *http.Request) {
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

		err = addTunByModel(mgr, data)
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

func addTunByModel(mgr *tun.GroupManager, m model.Tunnel) error {
	cfg, err := Model2Config(&m)
	if err != nil {
		return err
	}

	now := time.Now()
	cfg.CreatedAt = now

	err = mgr.AddService(*cfg, true)
	if err != nil {
		return err
	}
	return nil
}

func Model2Config(ls *model.Group) (*tun.GroupConfig, error) {
	var result tun.GroupConfig
	err := json.Unmarshal([]byte(ls.Cfg), &result)
	return &result, err
}

// --TODO
func Group2Model(group *tun.Group) (model.Group, error) {
	cfg := group.Cfg()
	data, err := json.Marshal(cfg)
	if err != nil {
		return model.Group{}, err
	}

	return model.Group{
		UUID:      cfg.UUID,
		Name:      cfg.Name,
		Input:     cfg.Input.Addr,
		Output:    "",
		Outputs:   "",
		Status:    group.Status(),
		CreatedAt: ParseTime2String(cfg.CreatedAt),
		Cfg:       string(data),
	}, nil
}

func ParseTime2String(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func Edit(mgr *tun.GroupManager) func(writer http.ResponseWriter, request *http.Request) {
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

		err = mgr.RemoveServiceByUUID(cfg.UUID)
		if err != nil {
			panic(err.Error())
		}

		err = addTunByModel(mgr, data)
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

func Delete(mgr *tun.GroupManager) func(writer http.ResponseWriter, request *http.Request) {
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

func Import(mgr *tun.GroupManager) func(writer http.ResponseWriter, request *http.Request) {
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

		cfg := &model.Tunnel{}
		err = json.Unmarshal(body, cfg)
		if err != nil {
			panic(err.Error())
		}

		err = addTunByModel(mgr, *cfg)
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

func Export(mgr *tun.GroupManager) func(w http.ResponseWriter, request *http.Request) {
	return func(w http.ResponseWriter, request *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				msg, _ := json.Marshal(response.Ret{
					Code: http.StatusInternalServerError,
					Msg:  fmt.Sprintf("%v", err),
				})
				w.Write(msg)
			}
		}()

		name := request.FormValue("name")

		_, exist := mgr.GetService(name)
		if !exist {
			panic(errors.New("tun not exist"))
		}

		path := mgr.ServiceFile(name)

		filename := filepath.Base(path)

		file, err := os.Open(path)
		if err != nil {
			panic(err.Error())
		}
		defer file.Close()

		fileHeader := make([]byte, 512)
		file.Read(fileHeader)

		fileStat, _ := file.Stat()

		w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
		w.Header().Set("Content-Type", http.DetectContentType(fileHeader))
		w.Header().Set("Content-Length", strconv.FormatInt(fileStat.Size(), 10))

		file.Seek(0, 0)
		io.Copy(w, file)
	}
}

func CheckServer(mgr *tun.GroupManager) func(w http.ResponseWriter, request *http.Request) {
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

		request.ParseForm()

		serverType := request.FormValue("serverType")
		targetAddr := request.FormValue("targetAddr")

		now := time.Now()

		var result string
		switch serverType {
		case "echo":
			req := "hello"
			response, err := echo.CheckTCP(targetAddr, req, time.Second*2)
			elapseMS := time.Since(now).Milliseconds()
			if err != nil {
				result += fmt.Sprintf("tcp failed:%s,elapse:%dms \n", err.Error(), elapseMS)
			} else {
				if response != req {
					result += fmt.Sprintf("tcp failed,RTT:%dms,req:%s,resp:%s \n", elapseMS, req, response)
				} else {
					result += fmt.Sprintf("tcp passed,RTT:%dms,req:%s,resp:%s \n", elapseMS, req, response)
				}
			}

			now = time.Now()
			response, err = echo.CheckUDP(targetAddr, req, time.Second*2)
			elapseMS = time.Since(now).Milliseconds()

			if err != nil {
				result += fmt.Sprintf("udp failed:%s,elapse:%dms\n", err.Error(), elapseMS)
			} else {
				if response != req {
					result += fmt.Sprintf("udp failed,RTT:%dms,req:%s,resp:%s \n", elapseMS, req, response)
				} else {
					result += fmt.Sprintf("udp passed,RTT:%dms,req:%s,resp:%s \n", elapseMS, req, response)
				}
			}
		case "socks5":
			clientCfg, testWebUrl, err := socks5client.ParseUrl(targetAddr)
			if err != nil {
				result += fmt.Sprintf("parse socks5 addr failed:%s \n", err.Error())
				break
			}

			response, err := socks5client.CheckTCP(clientCfg, testWebUrl, time.Second*2)
			elapseMS := time.Since(now).Milliseconds()
			if err != nil {
				result += fmt.Sprintf("tcp failed:%s,elapse:%dms \n", err.Error(), elapseMS)
			} else {
				result += fmt.Sprintf("tcp passed,elapse:%dms,%s\n", elapseMS, response)
			}

			now = time.Now()
			advertisedUDPAddr, response, err := socks5client.CheckUDP(clientCfg, time.Second*5)
			elapseMS = time.Since(now).Milliseconds()
			if err != nil {
				result += fmt.Sprintf("udp failed,elapse:%dms,\nadvertised_addr:%s,err:%s", elapseMS, advertisedUDPAddr, err.Error())
			} else {
				result += fmt.Sprintf("udp passed,elapse:%dms,\nadvertised_addr:%s,rsp(8.8.8.8):%s", elapseMS, advertisedUDPAddr, response)
			}
		case "httpproxy":
			response, err := httpproxy.Check(targetAddr, time.Second*2)
			elapseMS := time.Since(now).Milliseconds()
			if err != nil {
				result += fmt.Sprintf("failed:%s,elapse:%dms", err.Error(), elapseMS)
			} else {
				result += fmt.Sprintf("passed,RTT:%dms,response(ipinfo.io):%s", elapseMS, response)
			}
		default:
			panic(errors.New("server type not support"))
		}

		ret := response.Ret{
			Code: http.StatusOK,
			Msg:  result,
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
