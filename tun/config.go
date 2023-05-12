package tun

type Config struct {
	Id     int32  `json:"id"`
	Name   string `json:"name"`
	Input  string `json:"input"`
	Output string `json:"output"`

	InDecryptKey  string `json:"in_decrypt_key"`
	InDecryptMode string `json:"in_decrypt_mode"`
	InExtra       string `json:"in_extra"`
	OutCryptKey   string `json:"out_crypt_key"`
	OutCryptMode  string `json:"out_crypt_mode"`
	OutExtra      string `json:"out_extra"`
	OutMuxConn    int32  `json:"out_mux_conn"`
}

type TCPConfig struct {
	NoMux bool `json:"no_mux"`
}

type KCPConfig struct {
	NoMux        bool `json:"no_mux"`
	StreamMode   bool `json:"stream_mode"`
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

type UDPConfig struct {
	Timeout int `json:"timeout"`
}
