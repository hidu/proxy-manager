proxy-manager
============

manager http、socks4、socks4a、socks5 proxy

auto check proxy alive  



download binary here(linux_x86_64 and windows_32): <http://pan.baidu.com/s/1c0dALWk#path=%252Fproxy-manager>  


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