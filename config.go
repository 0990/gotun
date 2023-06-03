package gotun

import (
	"encoding/json"
	"os"
)

type AppConfig struct {
	ListenPort int      `json:"listen_port"`
	LogLevel   string   `json:"log_level"`
	Admin      AdminCfg `json:"admin"`
	EchoPort   int      `json:"echo_port"`
	PProfPort  int      `json:"pprof_port"`
}

type AdminCfg struct {
	Username string `json:"username"` //账号
	Password string `json:"password"` //密码
}

func parseAppConfigFile(fileName string) (*AppConfig, error) {
	data, err := os.ReadFile(fileName)
	if err != nil {
		return nil, err
	}

	var cfg AppConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func createAppConfigFile(fileName string) error {
	cfg := AppConfig{
		ListenPort: 8080,
		LogLevel:   "info",
		Admin: AdminCfg{
			Username: "admin",
			Password: "admin",
		},
		PProfPort: 6060,
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(fileName, data, 0666)
}
