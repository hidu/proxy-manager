proxy-manager
============
v0.2.1  

##概述
1.  统一管理 http、socks4、socks4a、socks5、shadowsocks 代理
2.  自动检查代理是否可用
3.  对外统一提供http代理服务
4.  对外代理服务支持http basic认证

##安装
###使用源码安装
需要安装[golang](https://golang.org/dl/  "下载安装")
```
export GO15VENDOREXPERIMENT=1
```
```
go get -u github.com/hidu/proxy-manager
```

###下载二进制文件
> [网盘下载:windows、linux、darwin版本](http://pan.baidu.com/s/1c0dALWk)

##配置
###初始化配置
```
proxy-manager -init_conf ./conf/
```
###配置文件
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



##运行
```
proxy-manager -conf ./conf/proxy.conf
```


##流程图
用使用代理来访问 `http://www.baidu.com/` 来做示例：  
```

++++++++++++++++++++++++++++++++++++++++++++++++++++++++++  
+ client (want visit http://www.baidu.com/)              +  
++++++++++++++++++++++++++++++++++++++++++++++++++++++++++  
                        |  
                        |  via proxy 127.0.0.1:8090  
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