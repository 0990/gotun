package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"time"
)

// SERVICE_CONFIG_SUFFIX 内置服务实例配置文件后缀，每个实例一个文件，存放于 tunnel 目录
const SERVICE_CONFIG_SUFFIX = ".service"

// 内置服务类型
const (
	TypeEcho      = "echo"          // echo 测试服务
	TypeHttpProxy = "http_proxy"    // http 代理服务
	TypeSocks5    = "socks5_server" // 标准 socks5 服务
	TypeSocks5X   = "socks5x_server" // socks5x 服务（自定义协议）
)

// Config 内置服务实例配置
type Config struct {
	UUID      string    `json:"uuid"`
	Name      string    `json:"name"`
	Type      string    `json:"type"` // echo|http_proxy|socks5_server|socks5x_server
	Disabled  bool      `json:"disabled"`
	CreatedAt time.Time `json:"create_at"`

	Listen     string `json:"listen"`      // echo / http_proxy 监听地址，如 0.0.0.0:8081
	ListenPort int    `json:"listen_port"` // socks5 / socks5x 监听端口
	TCPTimeout int    `json:"tcp_timeout"` // socks5 / socks5x tcp 超时（秒）
	UDPTimeout int    `json:"udp_timeout"` // socks5 / socks5x udp 超时（秒）
	Username   string `json:"username,omitempty"` // socks5 可选认证账号
	Password   string `json:"password,omitempty"` // socks5 可选认证密码
}

// serviceFile 返回某内置服务实例的持久化文件路径
func serviceFile(dir string, name string) string {
	return dir + "/" + name + SERVICE_CONFIG_SUFFIX
}

// createServiceFile 原子写入 service 配置文件（已存在则报错）
func createServiceFile(dir string, cfg Config) error {
	return writeServiceFileAtomic(dir, cfg)
}

// deleteServiceFile 删除 service 配置文件（不存在则视为成功）
func deleteServiceFile(dir string, name string) error {
	err := os.Remove(serviceFile(dir, name))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return nil
}

// writeServiceFileAtomic 原子写入 service 配置文件（先写临时文件再 rename）
func writeServiceFileAtomic(dir string, cfg Config) error {
	if _, err := serviceStageDir(dir); err != nil {
		return err
	}

	filename := serviceFile(dir, cfg.Name)
	if isFileExist(filename) {
		return errors.New("service already exist")
	}

	stageDir := filepath.Join(dir, ".staging")
	tempFile, err := writeTempServiceFile(stageDir, cfg)
	if err != nil {
		return err
	}
	defer os.Remove(tempFile)

	return os.Rename(tempFile, filename)
}

// replaceServiceFile 用新配置替换旧配置（带 .bak 备份与回滚）
func replaceServiceFile(dir string, oldCfg Config, newCfg Config) error {
	stageDir, err := serviceStageDir(dir)
	if err != nil {
		return err
	}

	newTemp, err := writeTempServiceFile(stageDir, newCfg)
	if err != nil {
		return err
	}
	defer os.Remove(newTemp)

	oldPath := serviceFile(dir, oldCfg.Name)
	newPath := serviceFile(dir, newCfg.Name)
	backupPath := filepath.Join(stageDir, fmt.Sprintf("%s-%d.service.bak", oldCfg.Name, time.Now().UnixNano()))

	oldExists := isFileExist(oldPath)
	if oldExists {
		if err := os.Rename(oldPath, backupPath); err != nil {
			return err
		}
	}

	if err := os.Rename(newTemp, newPath); err != nil {
		if oldExists {
			_ = os.Rename(backupPath, oldPath)
		}
		return err
	}

	if oldExists {
		_ = os.Remove(backupPath)
	}
	return nil
}

// writeTempServiceFile 把 service 配置写入临时文件，返回临时文件路径
func writeTempServiceFile(stageDir string, cfg Config) (string, error) {
	cfgData, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}

	f, err := os.CreateTemp(stageDir, cfg.Name+".*.service.tmp")
	if err != nil {
		return "", err
	}

	if _, err := f.Write(cfgData); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return f.Name(), nil
}

// serviceStageDir 确保目录与 .staging 暂存目录存在
func serviceStageDir(dir string) (string, error) {
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return "", err
	}

	stageDir := filepath.Join(dir, ".staging")
	if err := os.MkdirAll(stageDir, os.ModePerm); err != nil {
		return "", err
	}
	return stageDir, nil
}

// loadAllServiceFile 加载目录下所有 .service 配置
func loadAllServiceFile(dir string) ([]Config, error) {
	rd, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var cfgs []Config
	for _, v := range rd {
		if v.IsDir() {
			continue
		}
		name := v.Name()

		if path.Ext(name) != SERVICE_CONFIG_SUFFIX {
			continue
		}

		data, err := os.ReadFile(dir + "/" + v.Name())
		if err != nil {
			return nil, err
		}

		var cfg Config
		err = json.Unmarshal(data, &cfg)
		if err != nil {
			return nil, err
		}
		cfgs = append(cfgs, cfg)
	}
	return cfgs, nil
}

func isFileExist(path string) bool {
	_, err := os.Stat(path)
	if err != nil {
		if os.IsExist(err) {
			return true
		}
		return false
	}
	return true
}
