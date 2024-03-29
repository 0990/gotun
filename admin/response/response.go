package response

type Ret struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data"`
}

type List struct {
	List       interface{} `json:"list"`
	Version    string      `json:"version"`
	Pagination Pagination  `json:"pagination"`
}

type Pagination struct {
	CurrentPage int `json:"current_page"`
	PageSize    int `json:"page_size"`
	TotalPages  int `json:"total_pages"`
	TotalNums   int `json:"total_nums"`
}
