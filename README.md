proxy-manager
============
v0.3.0

## 概述
1.  统一管理 http、https、socks4、socks4a、socks5、shadowsocks 代理
2.  自动检查代理是否可用
3.  对外统一提供http代理服务
4.  对外代理服务支持http basic认证
5.  支持通过接口添加代理

## 安装
### 使用源码安装
需要安装[Go](https://golang.org/dl/  "下载安装")
```
go install github.com/hidu/proxy-manager@master
```

## 配置
### 初始化配置
```
proxy-manager -init_conf
```
### 配置文件
<table>
<thead>
 <tr>
    <th>文件名</th>
    <th>说明</th>
 </tr>
</thead>
<tbody>
  <tr>
    <td>proxy.toml</td>
    <td>主配置文件</td>
  </tr>
  <tr>
    <td>pool.conf</td>
    <td>代理池，每行配置一个代理，每次启动都会加载检查</td>
  </tr>
  <tr>
    <td>pool_checked.list</td>
    <td>程序生成，当前检查可用的代理结果</td>
  </tr>
  <tr>
    <td>pool_bad.list</td>
    <td>程序生成，不可用的代理列表</td>
  </tr>
</tbody>
</table>



## 运行
```bash
proxy-manager

or

proxy-manager -conf ./conf/proxy.toml
```


## 使用流程
假设服务监听地址为：`127.0.0.1:8128`

### As Proxy Server
支持访问 http URL，暂不支持 https URL。
```
curl -x http://$name:$psw@127.0.0.1:8128 'http://hidu.github.io/hello.md'
```

### As Gateway Server
支持访问 http 和 https URL 。
```bash
# 发送 GET 请求
curl 'http://$name:$psw@127.0.0.1:8128/query?url=https://hidu.github.io/hello.md

# 发送 POST 请求，并且有设置自定义 Header 以及 Body 
curl 'http://$name:$psw@127.0.0.1:8128/query?method=POST&url=https://hidu.github.io/hello.md&headers={"a":["a"]}' \
  -X POST --data "request body"
```

获取一个 Proxy
```bash
 curl 'http://$name:$psw@127.0.0.1:8128/fetch'
```

成功的 Response：
```json
{
    "ErrNo": 0,
    "Proxy": "http://127.0.0.1:8101"
}
```

## 外部接口

### 添加代理接口
```
curl 命令示例：
curl --data "user_name=admin&psw_md5=7bb483729b5a8e26f73e1831cde5b842&proxy=http://10.0.1.9:3128" http://127.0.0.1:8128/add
```

### 服务状态接口
http://127.0.0.1:8128/status