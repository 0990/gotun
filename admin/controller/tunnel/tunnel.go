package tunnel

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"strconv"

	"github.com/0990/gotun/admin/config"
	"github.com/0990/gotun/admin/model"
	"github.com/0990/gotun/admin/response"
)

func List(writer http.ResponseWriter, request *http.Request) error {
	db := config.GlobalDbConnect
	if db == nil {
		return errors.New("db is nil")
	}
	// Get page
	err := request.ParseForm()
	if err != nil {
		return err
	}

	page := request.FormValue("page")
	if page == "" {
		page = "1"
	}

	pageInt, err := strconv.Atoi(page)
	if err != nil {
		return err
	}
	// Get search_data
	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		return err
	}

	filters := &model.Tunnel{}
	err = json.Unmarshal(body, &filters)
	if err != nil {
		return err
	}
	// Get total nums
	totalNums := 0
	pageSize := 20
	table := []model.Tunnel{}
	db.Where(filters).Find(&table).Count(&totalNums)

	// Compute total_pages
	totalPages := math.Ceil(float64(totalNums) / float64(pageSize))

	// Get records
	records := []model.Tunnel{}
	//db.Where(filters).Find(&records)
	db.Where(filters).Limit(pageSize).Offset((pageInt - 1) * pageSize).Order("created_at desc").Find(&records)

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
		return err
	}

	_, err = writer.Write(d)
	if err != nil {
		return err
	}

	return nil
}

func Delete(writer http.ResponseWriter, request *http.Request) error {
	db := config.GlobalDbConnect
	if db == nil {
		return errors.New("db is nil")
	}

	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		log.Printf("%v", err)
		return err
	}

	data := make(map[string]int32)
	err = json.Unmarshal(body, &data)
	if err != nil {
		return err
	}

	sql := model.Tunnel{
		Id: data["id"],
	}

	db.Delete(&sql)

	ret := response.Ret{
		Code: http.StatusOK,
		Msg:  "success",
	}

	d, err := json.Marshal(&ret)
	if err != nil {
		return err
	}

	_, err = writer.Write(d)
	if err != nil {
		return err
	}

	return nil
}

func Create(writer http.ResponseWriter, request *http.Request) error {
	db := config.GlobalDbConnect
	if db == nil {
		return errors.New("db is nil")
	}

	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		return err
	}

	data := model.Tunnel{}
	err = json.Unmarshal(body, &data)
	if err != nil {
		return err
	}

	db.Create(&data)

	ret := response.Ret{
		Code: http.StatusOK,
		Msg:  "success",
	}

	d, err := json.Marshal(&ret)
	if err != nil {
		return err
	}

	_, err = writer.Write(d)
	if err != nil {
		return err
	}

	return nil
}

func Edit(writer http.ResponseWriter, request *http.Request) error {
	db := config.GlobalDbConnect
	if db == nil {
		return errors.New("db is nil")
	}

	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		return err
	}

	data := model.Tunnel{}
	err = json.Unmarshal(body, &data)
	if err != nil {
		return err
	}

	updateData := model.Tunnel{}
	db.First(&updateData, data.Id)
	updateData = data
	db.Save(&updateData)

	ret := response.Ret{
		Code: http.StatusOK,
		Msg:  "success",
		Data: &data,
	}

	d, err := json.Marshal(&ret)
	if err != nil {
		return err
	}

	_, err = writer.Write(d)
	if err != nil {
		return err
	}

	return nil

}

func Detail(writer http.ResponseWriter, request *http.Request) error {
	db := config.GlobalDbConnect
	if db == nil {
		return errors.New("db is nil")
	}

	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		return err
	}

	log.Printf("%s", body)

	params := make(map[string]int)
	err = json.Unmarshal(body, &params)
	if err != nil {
		return err
	}

	data := model.Tunnel{}

	db.First(&data, params["id"])

	ret := response.Ret{
		Code: http.StatusOK,
		Msg:  "success",
		Data: &data,
	}

	d, err := json.Marshal(&ret)
	if err != nil {
		return err
	}

	_, err = writer.Write(d)
	if err != nil {
		return err
	}

	return nil
}

func BatchDelete(writer http.ResponseWriter, request *http.Request) error {
	db := config.GlobalDbConnect
	if db == nil {
		return errors.New("db is nil")
	}

	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		log.Printf("%v", err)
		return err
	}

	data := []int32{}
	err = json.Unmarshal(body, &data)
	if err != nil {
		return err
	}

	for _, id := range data {
		db.Where(model.Tunnel{Id: id}).Delete(model.Tunnel{})
	}

	ret := response.Ret{
		Code: http.StatusOK,
		Msg:  "success",
	}

	d, err := json.Marshal(&ret)
	if err != nil {
		return err
	}

	_, err = writer.Write(d)
	if err != nil {
		return err
	}

	return nil
}
