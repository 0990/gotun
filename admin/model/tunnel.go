package model

type QualitySummary struct {
	Status    string  `json:"status"`
	RTTMs     int64   `json:"rtt_ms"`
	LossPct   float64 `json:"loss_pct"`
	LastError string  `json:"last_error"`
}

type BandwidthSummary struct {
	Status    string  `json:"status"`
	Mbps      float64 `json:"mbps"`
	LastError string  `json:"last_error"`
	TestedAt  string  `json:"tested_at"`
}

type Tunnel struct {
	UUID     string `json:"uuid"`
	Name     string `json:"name"`
	Disabled bool   `json:"disabled"`
	Mode     string `json:"mode""` //工作模式 nil|frpc|frps frpc模式下 Input为worker,配置是输出模式;frps模式下 output为worker,配置是输入模式

	Input         string `json:"input"`
	InProtoCfg    string `json:"in_proto_cfg"`
	InDecryptMode string `json:"in_decrypt_mode"`
	InDecryptKey  string `json:"in_decrypt_key"`
	InExtend      string `json:"in_extend"`

	Output       string `json:"output"`
	OutProtoCfg  string `json:"out_proto_cfg"`
	OutCryptMode string `json:"out_crypt_mode"`
	OutCryptKey  string `json:"out_crypt_key"`
	OutExtend    string `json:"out_extend"`

	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`

	QualitySummary   QualitySummary   `json:"quality_summary"`
	BandwidthSummary BandwidthSummary `json:"bandwidth_summary"`
}

func (tunnel *Tunnel) TableName() string {
	return "tunnel"
}
