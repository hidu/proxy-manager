module github.com/hidu/proxy-manager

go 1.16

require (
	github.com/aead/chacha20 v0.0.0-20180709150244-8b13a72661da // indirect
	github.com/fsgo/fsconf v0.2.4
	github.com/fsgo/fsenv v0.3.0
	github.com/fsgo/fsgo v0.0.4
	github.com/hidu/goutils v0.0.3-0.20200404095852-11ec3e603b6e
	github.com/shadowsocks/shadowsocks-go v0.0.0-20200409064450-3e585ff90601
	golang.org/x/crypto v0.0.0-20200403201458-baeed622b8d8 // indirect
	golang.org/x/net v0.0.0-20220630215102-69896b714898
	h12.io/socks v1.0.3
)

replace h12.io/socks => github.com/h12w/socks v1.0.3
