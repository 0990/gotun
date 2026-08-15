package gotun

import (
	"os"

	"github.com/0990/gotun/server/service"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

// legacyBuiltIn 旧版 app.yaml 中 build-in 配置的结构，仅用于一次性迁移
type legacyBuiltIn struct {
	Enable          bool   `yaml:"enable"`
	EchoListen      string `yaml:"echo_listen"`
	HttpProxyListen string `yaml:"http_proxy_listen"`
	Socks5XServer   struct {
		ListenPort int `yaml:"listen_port"`
		UDPTimout  int `yaml:"udp_timeout"`
		TCPTimeout int `yaml:"tcp_timeout"`
	} `yaml:"socks5x_server"`
}

// migrateBuildInConfig 检测旧版 app.yaml 的 build-in 配置，迁移为 tunnel 目录下的 .service 实例文件，
// 并从 app.yaml 中移除 build-in 块。幂等：同名 .service 已存在则跳过写入。
// 说明：此处只负责写 .service 文件，实例统一由 svcMgr.Run() 加载启动，避免重复监听端口。
func migrateBuildInConfig(appFile string, svcMgr *service.Manager) error {
	data, err := os.ReadFile(appFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	// 宽松解析：只关心 build-in 键是否存在及其内容
	var raw struct {
		BuildIn *legacyBuiltIn `yaml:"build-in"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw.BuildIn == nil {
		// 无 build-in 配置，无需迁移
		return nil
	}

	in := raw.BuildIn
	if in.Enable {
		cfgs := buildInToServiceConfigs(in)
		for _, cfg := range cfgs {
			// 幂等：同名 .service 已存在则跳过；此处只写文件，实例由 svcMgr.Run() 统一加载启动
			if svcMgr.ServiceExistFile(cfg.Name) {
				continue
			}
			if err := svcMgr.CreateFileOnly(cfg); err != nil {
				logrus.WithError(err).WithField("service", cfg.Name).Error("迁移内置服务失败")
				continue
			}
			logrus.WithField("service", cfg.Name).WithField("type", cfg.Type).Info("已从 build-in 迁移内置服务")
		}
	}

	// 从 app.yaml 移除 build-in 块（保留其余键与顺序）
	if err := removeBuildInKey(appFile, data); err != nil {
		return err
	}
	return nil
}

// buildInToServiceConfigs 把旧 build-in 配置转成对应的 service.Config 列表
func buildInToServiceConfigs(in *legacyBuiltIn) []service.Config {
	var cfgs []service.Config

	if len(in.EchoListen) > 0 {
		cfgs = append(cfgs, service.Config{
			Name:   "echo",
			Type:   service.TypeEcho,
			Listen: in.EchoListen,
		})
	}
	if len(in.HttpProxyListen) > 0 {
		cfgs = append(cfgs, service.Config{
			Name:   "http_proxy",
			Type:   service.TypeHttpProxy,
			Listen: in.HttpProxyListen,
		})
	}
	if in.Socks5XServer.ListenPort > 0 {
		cfgs = append(cfgs, service.Config{
			Name:       "socks5x",
			Type:       service.TypeSocks5X,
			ListenPort: in.Socks5XServer.ListenPort,
			TCPTimeout: in.Socks5XServer.TCPTimeout,
			UDPTimeout: in.Socks5XServer.UDPTimout,
		})
	}
	return cfgs
}

// removeBuildInKey 从 yaml 文本中删除 build-in 顶层键并写回（保留其余键与注释）
func removeBuildInKey(appFile string, data []byte) error {
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return err
	}

	// 根节点是 DocumentNode，其子节点为 MappingNode
	if len(node.Content) == 0 {
		return nil
	}
	mapping := node.Content[0]
	if mapping.Kind != yaml.MappingNode {
		return nil
	}

	// 过滤掉 build-in 键值对（Content 为 key,value 交替）
	filtered := make([]*yaml.Node, 0, len(mapping.Content))
	for i := 0; i < len(mapping.Content); i += 2 {
		key := mapping.Content[i]
		if key.Value == "build-in" {
			continue
		}
		filtered = append(filtered, mapping.Content[i], mapping.Content[i+1])
	}
	mapping.Content = filtered

	out, err := yaml.Marshal(&node)
	if err != nil {
		return err
	}
	return os.WriteFile(appFile, out, 0666)
}
