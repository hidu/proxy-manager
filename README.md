# proxy-manager

[中文](https://github.com/hidu/proxy-manager)   |
[Deutsch](https://zdoc.app/de/hidu/proxy-manager) |
[English](https://zdoc.app/en/hidu/proxy-manager) |
[Español](https://zdoc.app/es/hidu/proxy-manager) |
[français](https://zdoc.app/fr/hidu/proxy-manager) |
[日本語](https://zdoc.app/ja/hidu/proxy-manager) |
[한국어](https://zdoc.app/ko/hidu/proxy-manager) |
[Português](https://zdoc.app/pt/hidu/proxy-manager) |
[Русский](https://zdoc.app/ru/hidu/proxy-manager) 


## 概述
1.  统一管理 http、https、socks4、socks4a、socks5、shadowsocks 代理
2.  自动检查代理是否可用
3.  对外统一提供 HTTP/HTTPS 代理服务
4.  对外代理服务支持 HTTP Basic 认证
5.  支持通过接口添加代理地址、获取可用代理

## 安装
使用源码安装:
```
go install github.com/hidu/proxy-manager@master
```

或者在 [ releases 页面](https://github.com/hidu/proxy-manager/releases) 下载编译好的二进制。

## 配置
### 配置文件
 参考项目 [conf](./conf) 目录内的配置。

## 运行
```bash
proxy-manager

or

proxy-manager -conf ./conf/app.yml
```


## 使用
 1. 将固定的代理配置添加到 `conf/proxies.yml` 文件中 (每次重启后都会加载)
 2. 在 `conf/users.yml` 中配置用户 (登录管理页面 和 使用代理时会用) 
 3. 启动服务，服务监听地址默认为：`127.0.0.1:8128`
 4. 在浏览器中访问 `http://127.0.0.1:8128` 可以进入管理页面 

`http://127.0.0.1:8128` 即提供了管理页面，同时也提供了 HTTP / HTTPS 代理功能。

### 作为 HTTP / HTTPS 代理
比如：
```
# 在 conf/app.yml 中配置了 AuthType="no" （不需要代理认证）时：
curl -x http://127.0.0.1:8128 'https://hidu.github.io/hello.md'
```
或者
```
# 在 conf/app.yml 中配置了 AuthType="basic" 或 "basic_any" （需要代理认证）时：
curl -x http://$name:$psw@127.0.0.1:8128 'https://hidu.github.io/hello.md'
```


### API

#### /query: 作为普通服务，转发请求

```bash
# 发送 GET 请求
curl 'http://$name:$psw@127.0.0.1:8128/query?url=https://hidu.github.io/hello.md

# 发送 POST 请求，并且有设置自定义 Header 以及 Body 
curl 'http://$name:$psw@127.0.0.1:8128/query?method=POST&url=https://hidu.github.io/hello.md&header={"k1":"v1"}' \
  -X POST --data "request body"
```

#### /fetch: 返回一个可用的代理服务器
```bash
 curl 'http://$name:$psw@127.0.0.1:8128/fetch'
```


#### /add: 添加代理
```bash
curl --data "user_name=admin&psw_md5=7bb483729b5a8e26f73e1831cde5b842&proxy=http://10.0.1.9:3128" http://127.0.0.1:8128/add
```

#### /status: 服务状态接口
```bash
curl http://127.0.0.1:8128/status
```
