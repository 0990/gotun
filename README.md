# gotun
tcp/udp转发器，可构建加密安全通道

## Feature
* 支持tcp,udp,quic,kcp流量转发
* 支持构建加密安全通道，可使用tcp,udp,quic,kcp作通道传输协议
* 支持内网穿透式安全通道(重构中)
* web界面管理

## 使用
点此下载二进制文件，启动即可（会自动生成配置app.yaml）
```yaml
# web监听地址
web_listen: 0.0.0.0:8080
# web登录账号
web_username: admin
# web登录密码
web_password: admin
# 日志等级:debug/info/warn/error
log_level: info
# echo服务监听地址,用于测试，客户端向此端口发送什么就回什么，可为空，为空则不启动echo服务
echo_listen: 0.0.0.0:8081
# pprof监听地址,可为空
pprof_port: ""
```
访问[127.0.0.1:8080](http://127.0.0.1:8080),输入默认账号密码admin/admin登录<br>
## tcp/udp等转发服务
## 需求
默认echo服务的地址是127.0.0.1:8081<br>
希望通过127.0.0.1:8888转发127.0.0.1:8081，访问127.0.0.1:8888就相当于访问echo服务
### 创建
通过New按钮创建一个tcp转发，配置如下：
```json
{
    "name": "tcp relayer",
    "input": "tcp@0.0.0.0:8888",
    "output": "tcp@127.0.0.1:8081",
    "mode": "",
    "in_proto_cfg": "{\"head_trim\":\"\"}",
    "in_decrypt_mode": "",
    "in_decrypt_key": "",
    "in_extend": "{\"mux_conn\":0}",
    "out_proto_cfg": "{\"head_append\":\"\"}",
    "out_crypt_mode": "",
    "out_crypt_key": "",
    "out_extend": "{\"mux_conn\":0}"
}
```
注意，为了测试方便，output指向的默认配置自带的echo服务的地址
## 测试
1. echo服务：nc 127.0.0.1:8081,输入任意字符，收到相同的字符,代表echo服务正常运行<br>
2. 转发服务：nc 127.0.0.1:8888,输入任意字符，会收到相同的字符,代表转发服务OK<br>

## 安全通道服务
### 需求
默认echo服务的地址是127.0.0.1:8081<br>
但是访问echo服务都是明文，会泄露信息，希望通过加密通道访问echo服务<br>
最终效果是访问127.0.0.1:8888就相当于访问echo服务,中间的通信是加密的

### 创建
希望通过tcpMux作为加密通道，加密方式为gcm，加密key为goodweather<br>
需要创建一对服务，一个作为客户端：
```json
{
  "name": "加密通道客户端",
  "input": "tcp@0.0.0.0:8888",
  "output": "tcp_mux@127.0.0.1:8889",
  "mode": "",
  "in_proto_cfg": "{\"head_trim\":\"\"}",
  "in_decrypt_mode": "",
  "in_decrypt_key": "",
  "in_extend": "{\"mux_conn\":0}",
  "out_proto_cfg": "{\"head_append\":\"\"}",
  "out_crypt_mode": "gcm",
  "out_crypt_key": "goodweather",
  "out_extend": "{\"mux_conn\":10}"
}
```
一个作为服务端：
```json
{
  "name": "加密通道服务端",
  "input": "tcp_mux@0.0.0.0:8889",
  "output": "tcp@127.0.0.1:8081",
  "mode": "",
  "in_proto_cfg": "{\"head_trim\":\"\"}",
  "in_decrypt_mode": "",
  "in_decrypt_key": "",
  "in_extend": "{\"mux_conn\":0}",
  "out_proto_cfg": "{\"head_append\":\"\"}",
  "out_crypt_mode": "gcm",
  "out_crypt_key": "goodweather",
  "out_extend": "{\"mux_conn\":0}",
  "create_at": "2023-06-04T16:30:13.4126077+08:00"
}
```
要点：
* 客户端的output需要指向服务端的input，两边的协议、加密方式和加密key需要一致
* 加密通道协议可以是tcp,tcpmux,quic,kcp,kcpmux

注意，为了测试方便，服务端output指向的默认配置自带的echo服务的地址
## 测试
1. echo服务：nc 127.0.0.1:8081,输入任意字符，收到相同的字符,代表echo服务正常运行<br>
2. 转发服务：nc 127.0.0.1:8888,输入任意字符，会收到相同的字符,代表转发服务OK<br>

## 内网穿透式安全通道（重构中）
### 需求
默认echo服务的地址是127.0.0.1:8081<br>
但是访问echo服务都是明文，会泄露信息，希望通过加密通道访问echo服务<br>
最终效果是访问127.0.0.1:8888就相当于访问echo服务,中间的通信是加密的，与上面的安全通道不同的是，而是通过内网穿透的方式连接的
我们假设echo服务127.0.0.1:8081是在内网中，而127.0.0.1:8888,8889是在公网中,我们希望通过公网的8888访问内网的8081

### 创建
希望通过tcpMux作为加密通道，加密方式为gcm，加密key为goodweather<br>
需要创建一对服务，一个作为客户端,放置在echo服务内网中：
```json
{
  "name": "内网穿透客户端",
  "input": "tcpmux@127.0.0.1:8889",
  "output": "tcp@127.0.0.1:8081",
  "mode": "frpc",
  "in_proto_cfg": "{\"head_append\":\"\"}",
  "in_decrypt_mode": "gcm",
  "in_decrypt_key": "goodweather",
  "in_extend": "{\"mux_conn\":10}",
  "out_proto_cfg": "{\"head_append\":\"\"}",
  "out_crypt_mode": "",
  "out_crypt_key": "",
  "out_extend": "{\"mux_conn\":0}",
  "create_at": "2023-06-04T16:46:00.0074843+08:00"
}
```
一个作为服务端，放置在8888公网中：
```json
{
  "name": "内网穿透服务端",
  "input": "tcp@0.0.0.0:8888",
  "output": "tcp_mux@127.0.0.1:8889",
  "mode": "frps",
  "in_proto_cfg": "{\"head_trim\":\"\"}",
  "in_decrypt_mode": "",
  "in_decrypt_key": "",
  "in_extend": "{\"mux_conn\":0}",
  "out_proto_cfg": "{\"head_trim\":\"\"}",
  "out_crypt_mode": "gcm",
  "out_crypt_key": "goodweather",
  "out_extend": "{\"mux_conn\":0}",
  "create_at": "2023-06-04T16:48:05.5282116+08:00"
}
```
要点：
* 客户端的input需要指向服务端的output，两边的协议、加密方式和加密key需要一致
* 加密通道协议可以是tcp,tcpmux,quic,kcp,kcpmux

注意，为了测试方便，客户端output指向的默认配置自带的echo服务的地址
## 测试
1. echo服务：nc 127.0.0.1:8081,输入任意字符，收到相同的字符,代表echo服务正常运行<br>
2. 转发服务：nc 127.0.0.1:8888,输入任意字符，会收到相同的字符,代表转发服务OK<br>


