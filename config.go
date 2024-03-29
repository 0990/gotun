package gotun

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"path"
)

type AppConfig struct {
	WebListen               string `yaml:"web_listen"`                   //监听地址
	WebUsername             string `yaml:"web_username"`                 //账号
	WebPassword             string `yaml:"web_password"`                 //密码
	WebLoginFailLimitInHour int    `yaml:"web_login_fail_limit_in_hour"` //每小时最大失败次数

	LogLevel         string `yaml:"log_level"`
	PProfListen      string `yaml:"pprof_listen"`
	PrometheusListen string `yaml:"prometheus_listen"`

	BuildIn BuiltIn `yaml:"build-in"` //内置的服务
}

type BuiltIn struct {
	Enable          bool                `yaml:"enable"`
	EchoListen      string              `yaml:"echo_listen"`
	HttpProxyListen string              `yaml:"http_proxy_listen"`
	Socks5XServer   Socks5XServerConfig `yaml:"socks5x_server"`
}

type Socks5XServerConfig struct {
	ListenPort int `yaml:"listen_port"`
	UDPTimout  int `yaml:"udp_timeout"`
	TCPTimeout int `yaml:"tcp_timeout"`
}

func parseAppConfigFile(fileName string) (*AppConfig, error) {
	data, err := os.ReadFile(fileName)
	if err != nil {
		return nil, err
	}

	var cfg AppConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func createAppConfigFile(fileName string) (*AppConfig, error) {
	buildIn := BuiltIn{
		Enable:          false,
		EchoListen:      "0.0.0.0:8081",
		HttpProxyListen: "0.0.0.0:3128",
		Socks5XServer: Socks5XServerConfig{
			ListenPort: 1080,
			UDPTimout:  120,
			TCPTimeout: 120,
		},
	}

	cfg := AppConfig{
		WebListen:               "0.0.0.0:8080",
		WebUsername:             "admin",
		WebPassword:             "admin",
		WebLoginFailLimitInHour: 10,
		LogLevel:                "info",
		PProfListen:             "",
		BuildIn:                 buildIn,
	}

	node := &yaml.Node{
		Kind: yaml.MappingNode,
	}

	err := node.Encode(&cfg)
	if err != nil {
		return nil, err
	}
	addComments(node)
	data, err := yaml.Marshal(node)
	if err != nil {
		return nil, err
	}

	os.MkdirAll(path.Dir(fileName), os.ModePerm)

	err = os.WriteFile(fileName, data, 0666)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

func addComments(node *yaml.Node) {
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]

		switch key.Value {
		case "web_listen":
			key.HeadComment = "web监听地址"
		case "web_username":
			key.HeadComment = "web登录账号"
		case "web_password":
			key.HeadComment = "web登录密码"
		case "web_login_fail_limit_in_hour":
			key.HeadComment = "每小时登录失败限制次数"
		case "log_level":
			key.HeadComment = "日志等级:debug/info/warn/error"
		case "pprof_listen":
			key.HeadComment = "pprof监听地址,可为空"
		case "prometheus_listen":
			key.HeadComment = "prometheus监听地址,可为空"
		case "build-in":
			key.HeadComment = "内置服务配置"

			subNode := node.Content[i+1]
			for j := 0; j < len(subNode.Content); j += 2 {
				key2 := subNode.Content[j]
				switch key2.Value {
				case "enable":
					key2.HeadComment = "是否启用内置服务,总开关，false情况下不启用(会忽略下面的配置)"
				case "socks5x_server":
					key2.HeadComment = "socks5x服务配置,为空则不启动"
				case "echo_listen":
					key2.HeadComment = "echo服务监听地址,用于测试，客户端向此端口发送什么就回什么，为空则不启动"
				case "http_proxy_listen":
					key2.HeadComment = "http代理服务监听地址,为空则不启动"
				}
			}
		}
	}
}

func Welcome(appCfg *AppConfig) {
	str := "\nSTART-------------------------------------\n" +
		"Enjoy your system ^ ^" +
		"\nGenerated by Go-sword" +
		"\nhttps://github.com/sunshinev/go-sword" +
		"\n[Server info]" +
		"\nPProf Listen : " + appCfg.PProfListen +
		"\nPrometheus Listen : " + appCfg.PrometheusListen +
		"\nLog Level    : " + appCfg.LogLevel +
		"\n\nStart successful, server is running ...\n" +
		"Please request: " +
		appCfg.WebListen +
		"\nEND-------------------------------------\n"

	fmt.Println(str)
}
