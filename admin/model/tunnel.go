package model

type Tunnel struct {
	Name   string `json:"name"`
	Input  string `json:"input"`
	Output string `json:"output"`
	Mode   string `json:"mode""` //工作模式 nil|frpc|frps frpc模式下 Input为worker,配置是输出模式;frps模式下 output为worker,配置是输入模式

	InProtoCfg    string `json:"in_proto_cfg"`
	InDecryptMode string `json:"in_decrypt_mode"`
	InDecryptKey  string `json:"in_decrypt_key"`
	InExtend      string `json:"in_extend"`

	OutProtoCfg  string `json:"out_proto_cfg"`
	OutCryptMode string `json:"out_crypt_mode"`
	OutCryptKey  string `json:"out_crypt_key"`
	OutExtend    string `json:"out_extend"`

	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

func (tunnel *Tunnel) TableName() string {
	return "tunnel"
}
