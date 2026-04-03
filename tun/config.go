package tun

import (
	"encoding/json"
	"errors"
	"os"
	"path"
	"time"
)

const TUN_CONFIG_SUFFIX = ".tun"

type Config struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
	Mode string `json:"mode""` //工作模式 nil|frpc|frps frpc模式下 Input为worker,配置是输出模式;frps模式下 output为worker,配置是输入模式

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

	CreatedAt time.Time `json:"create_at"`
}

type Extend struct {
	MuxConn           int  `json:"mux_conn"`
	AutoExpire        int  `json:"auto_expire"`
	FrameHeaderEnable bool `json:"frame_header_enable"`
	ProbeIntervalSec  int  `json:"probe_interval_sec"`
	ProbeTimeoutMS    int  `json:"probe_timeout_ms"`
	ProbeWindowSize   int  `json:"probe_window_size"`
}

type InProtoTCP struct {
	Head string `json:"head"` //头部字段匹配删除
}

type OutProtoTCP struct {
	Head string `json:"head"` //头部数据填充
}

type InProtoTCPMux struct {
	Head string `json:"head"` //头部字段匹配删除
}

type OutProtoTCPMux struct {
	Head string `json:"head"` //头部数据填充
}

type InProtoSocks5X struct {
	UserName   string `json:"username"`
	Password   string `json:"password"`
	TCPTimeout int32  `json:"tcp_timeout"`

	UDPAdvertisedIP   string `json:"udp_advertised_ip"`
	UDPAdvertisedPort int    `json:"udp_advertised_port"`
}

var defaultInSocks5XConfig = InProtoSocks5X{
	UserName:          "",
	Password:          "",
	TCPTimeout:        300,
	UDPAdvertisedIP:   "",
	UDPAdvertisedPort: 0,
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
	WriteDelay:   true,
	MTU:          990,
	SndWnd:       2048,
	RcvWnd:       1024,
	DataShard:    0,
	ParityShard:  0,
	DSCP:         46,
	AckNodelay:   true,
	NoDelay:      0,
	Interval:     30,
	Resend:       0,
	NoCongestion: 0,
	SockBuf:      16777217,
	StreamBuf:    4194304,
}

type UDPConfig struct {
	Timeout int `json:"timeout"`
}

func createServiceFile(dir string, cfg Config) error {
	cfgData, err := json.Marshal(cfg)
	if err != nil {
		return err
	}

	filename := serviceFile(dir, cfg.Name)
	os.Mkdir(dir, os.ModePerm)

	if isFileExist(filename) {
		return errors.New("tun already exist")
	}

	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Write(cfgData)
	if err != nil {
		return err
	}
	return nil
}

func deleteServiceFile(dir string, name string) error {
	filename := serviceFile(dir, name)
	err := os.Remove(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return nil
}

func serviceFile(dir string, name string) string {
	return dir + "/" + name + TUN_CONFIG_SUFFIX
}

func loadAllServiceFile(dir string) ([]Config, error) {
	return loadAllFile[Config](dir, TUN_CONFIG_SUFFIX)
}

func loadAllFile[T any](dir string, suffix string) ([]T, error) {
	rd, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var cfgs []T
	for _, v := range rd {
		if v.IsDir() {
			continue
		}
		name := v.Name()

		suffix := path.Ext(name)
		if suffix != suffix {
			continue
		}

		data, err := os.ReadFile(dir + "/" + v.Name())
		if err != nil {
			return nil, err
		}

		var cfg T
		err = json.Unmarshal(data, &cfg)
		if err != nil {
			return nil, err
		}
		cfgs = append(cfgs, cfg)
	}
	return cfgs, nil
}

func isFileExist(path string) bool {
	_, err := os.Stat(path) //os.Stat获取文件信息
	if err != nil {
		if os.IsExist(err) {
			return true
		}
		return false
	}
	return true
}

type IOConfig struct {
	Addr      string `json:"addr"`
	ProtoCfg  string `json:"proto_cfg"`
	CryptMode string `json:"crypt_mode"`
	CryptKey  string `json:"crypt_key"`
	Extend    string `json:"extend"`
}
