module github.com/hidu/proxy-manager

go 1.14

require (
	github.com/Unknwon/goconfig v0.0.0-20191126170842-860a72fb44fd
	github.com/aead/chacha20 v0.0.0-20180709150244-8b13a72661da // indirect
	github.com/hidu/goutils v0.0.3-0.20200404095852-11ec3e603b6e
	github.com/shadowsocks/shadowsocks-go v0.0.0-20190614083952-6a03846ca9c0
	github.com/smartystreets/goconvey v0.0.0-20190731233626-505e41936337 // indirect
	golang.org/x/crypto v0.0.0-20200403201458-baeed622b8d8 // indirect
	golang.org/x/net v0.0.0-20200324143707-d3edc9973b7e
	golang.org/x/sys v0.0.0-20200331124033-c3d80250170d // indirect
	h12.io/socks v1.0.0
)

replace h12.io/socks => github.com/h12w/socks v1.0.0
