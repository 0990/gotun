package tun

type Config struct {
	Name   string `json:"name"`
	Input  string `json:"input"`
	Output string `json:"output"`
	Mode   string `json:"mode""` //工作模式 nil|frpc|frps frpc模式下 Input为worker,配置是输出模式;frps模式下 output为worker,配置是输入模式

	InProtoCfg    string `json:"in_proto_cfg"`
	InDecryptMode string `json:"in_decrypt_mode"`
	InDecryptKey  string `json:"in_decrypt_key"`
	InExtend      Extend `json:"in_extend"`

	OutProtoCfg  string `json:"out_proto_cfg"`
	OutCryptMode string `json:"out_crypt_mode"`
	OutCryptKey  string `json:"out_crypt_key"`
	OutExtend    Extend `json:"out_extend"`
}

type Extend struct {
	MuxConn int `json:"mux_conn"`
}

type InProtoTCP struct {
	HeadTrim []byte `json:"head_trim"` //头部字段匹配删除
}

type OutProtoTCP struct {
	HeadAppend []byte `json:"head_extend"` //头部数据填充
}

type InProtoTCPMux struct {
	HeadTrim []byte `json:"head_trim"` //头部字段匹配删除
}

type OutProtoTCPMux struct {
	HeadAppend []byte `json:"head_extend"` //头部数据填充
}

type QUICConfig struct {
}

type KCPConfig struct {
	WriteDelay   bool `json:"write_delay"`
	MTU          int  `json:"mtu"`
	SndWnd       int  `json:"sndwnd"`
	RcvWnd       int  `json:"rcvwnd"`
	DataShard    int  `json:"datashard"`
	ParityShard  int  `json:"parityshard"`
	DSCP         int  `json:"dscp"`
	AckNodelay   bool `json:"acknodelay"`
	NoDelay      int  `json:"nodelay"`
	Interval     int  `json:"interval"`
	Resend       int  `json:"resend"`
	NoCongestion int  `json:"nc"`
	SockBuf      int  `json:"sockbuf"`
	StreamBuf    int  `json:"streambuf"`
}

var defaultKCPConfig = KCPConfig{
	WriteDelay:   false,
	MTU:          1300,
	SndWnd:       2048,
	RcvWnd:       1024,
	DataShard:    10,
	ParityShard:  3,
	DSCP:         46,
	AckNodelay:   true,
	NoDelay:      0,
	Interval:     40,
	Resend:       0,
	NoCongestion: 0,
	SockBuf:      16777217,
	StreamBuf:    4194304,
}

type UDPConfig struct {
	Timeout int `json:"timeout"`
}
