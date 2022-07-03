proxy-manager
============
v0.2.3  

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
proxy-manager -init_conf ./conf/
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
    <td>proxy.conf</td>
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
```
proxy-manager -conf ./conf/proxy.conf
```


## 流程图
用使用代理来访问 `http://www.baidu.com/` 来做示例：  
```

++++++++++++++++++++++++++++++++++++++++++++++++++++++++++  
+ client (want visit http://www.baidu.com/)              +  
++++++++++++++++++++++++++++++++++++++++++++++++++++++++++  
                        |  
                        |  via proxy 127.0.0.1:8128  
                        |  
                        V  
++++++++++++++++++++++++++++++++++++++++++++++++++++++++++  
+                       +         proxy pool             +  
+ proxy manager listen  ++++++++++++++++++++++++++++++++++  
+ on (127.0.0.1:8090)   +  http_proxy1,http_proxy2,      +  
+                       +  socks5_proxy1,socks5_proxy2   +  
++++++++++++++++++++++++++++++++++++++++++++++++++++++++++  
                        |  
                        |  choose one proxy visit 
                        |  www.baidu.com  
                        |  
                        V  
++++++++++++++++++++++++++++++++++++++++++++++++++++++++++  
+        site:www.baidu.com                              +  
++++++++++++++++++++++++++++++++++++++++++++++++++++++++++  

```


## 外部接口

### 添加代理接口 
```
curl 命令示例：
curl --data "user_name=admin&psw_md5=7bb483729b5a8e26f73e1831cde5b842&proxy=http://10.0.1.9:3128" http://127.0.0.1:8128/add
```

### 服务状态接口
http://127.0.0.1:8128/status