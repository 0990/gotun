package model

type Tunnel struct {
	Id            int32  `gorm:"Column:id;primaryKey" json:"id"`
	InDecryptKey  string `gorm:"Column:in_decrypt_key" json:"in_decrypt_key"`
	InDecryptMode string `gorm:"Column:in_decrypt_mode" json:"in_decrypt_mode"`
	InExtra       string `gorm:"Column:in_extra" json:"in_extra"`
	Input         string `gorm:"Column:input" json:"input"`
	Name          string `gorm:"Column:name" json:"name"`
	OutCryptKey   string `gorm:"Column:out_crypt_key" json:"out_crypt_key"`
	OutCryptMode  string `gorm:"Column:out_crypt_mode" json:"out_crypt_mode"`
	OutExtra      string `gorm:"Column:out_extra" json:"out_extra"`
	OutMuxConn    int32  `gorm:"Column:out_mux_conn" json:"out_mux_conn"`
	Output        string `gorm:"Column:output" json:"output"`
}

func (tunnel *Tunnel) TableName() string {
	return "tunnel"
}
