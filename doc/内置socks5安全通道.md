## 内置socks5安全通道
### 需求
希望本地127.0.0.1:1080是socks5代理入口，请求通过加密通道转发到远程服务器192.168.1.30，远程服务器来处理具体请求<br>

### 创建
希望通过tcpMux作为加密通道，加密方式为gcm，加密key为goodweather<br>
需要创建一对服务，一个作为客户端,放在本地
```json
{
  "name": "socks5x客户端",
  "mode": "",
  "input": "socks5x@0.0.0.0:1080",
  "in_proto_cfg": "{\"username\":\"\",\"password\":\"\",\"tcp_timeout\":120,\"udp_advertised_ip\":\"\",\"udp_advertised_port\":0}",
  "in_decrypt_mode": "",
  "in_decrypt_key": "",
  "in_extend": "",
  "output": "tcp_mux@192.168.1.30:7779",
  "out_proto_cfg": "{\"head\":\"\"}",
  "out_crypt_mode": "gcm",
  "out_crypt_key": "goodweather",
  "out_extend": "{\"mux_conn\":10}"
}
```
一个作为服务端，放置在192.168.1.30机器中：
```json
{
  "name": "socks5x服务端",
  "mode": "",
  "input": "tcp_mux@0.0.0.0:7779",
  "in_proto_cfg": "{\"head\":\"\"}",
  "in_decrypt_mode": "gcm",
  "in_decrypt_key": "goodweather",
  "in_extend": "",
  "output": "tcp@127.0.0.1:1080",
  "out_proto_cfg": "{\"head_append\":\"\"}",
  "out_crypt_mode": "",
  "out_crypt_key": "",
  "out_extend": ""
}
```
要点：
* 客户端的input需要是socks5x
* 服务端需要开启内置的socks5x服务，output需要指向此服务

## 测试
客户端机器上执行
```bash
curl --proxy socks5://127.0.0.1:1080 ipinfo.io
```
或者 使用gotun检测命令,出现"check tcp success",代表成功
```bash
gotun --check_socks5 127.0.0.1:1080
````


## socks5 udp
### 需求
上面的服务只能处理tcp请求，如果需要处理udp请求，需要构建一个udp安全通道，指向上面socks5x的udp服务端口1080

### 创建
希望通过tcpMux作为加密通道，加密方式为gcm，加密key为goodweather<br>
需要创建一对服务，一个作为客户端,放在本地
```json
{
  "name": "udp客户端",
  "mode": "",
  "input": "udp@0.0.0.0:1081",
  "in_proto_cfg": "{\"timeout\":60}",
  "in_decrypt_mode": "",
  "in_decrypt_key": "",
  "in_extend": "",
  "output": "udp@192.168.1.30:7779",
  "out_proto_cfg": "{\"timeout\":60}",
  "out_crypt_mode": "gcm",
  "out_crypt_key": "goodweather",
  "out_extend": ""
}
```

一个作为服务端，放置在192.168.1.30机器中：
```json
{
    "name": "udp服务端",
    "mode": "",
    "input": "udp@0.0.0.0:7779",
    "in_proto_cfg": "{\"timeout\":60}",
    "in_decrypt_mode": "gcm",
    "in_decrypt_key": "goodweather",
    "in_extend": "",
    "output": "udp@127.0.0.1:1090",
    "out_proto_cfg": "{\"timeout\":60}",
    "out_crypt_mode": "",
    "out_crypt_key": "",
    "out_extend": ""
}
```

通道创建好后，还需要将"socks5x客户端"配置中的
* udp_advertised_ip设置为“udp客户端”的ip,这里是本机127.0.0.1
* udp_advertised_port设置为“udp客户端”的input端口,这里是1081

### 测试
使用gotun检测命令,出现"check udp success",代表成功
```bash
gotun --check_socks5 127.0.0.1:1080
````