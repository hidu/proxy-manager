proxy-manager
============

manager http、socks4、socks4a、socks5 proxy

auto check proxy alive  

install:
```
go get -u github.com/hidu/proxy-manager
```

proxy-manager provide http proxy with many proxies backend  

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
                        |choose one proxy visit www.baidu.com  
                        |  
                        V  
+++++++++++++++++++++++++++++++++++++++++++++++++++++++++  
+        site:www.baidu.com                             +  
+++++++++++++++++++++++++++++++++++++++++++++++++++++++++  

```