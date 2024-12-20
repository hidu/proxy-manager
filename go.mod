module github.com/hidu/proxy-manager

go 1.22.0

require (
	github.com/fsgo/fsconf v0.4.0
	github.com/fsgo/fsconf/confext v0.4.0
	github.com/fsgo/fsenv v0.6.0
	github.com/fsgo/fsgo v0.0.6
	github.com/hidu/goutils v0.0.3-0.20200404095852-11ec3e603b6e
	github.com/shadowsocks/shadowsocks-go v0.0.0-20200409064450-3e585ff90601
	golang.org/x/net v0.33.0
	h12.io/socks v1.0.3
)

require (
	github.com/BurntSushi/toml v1.4.0 // indirect
	github.com/aead/chacha20 v0.0.0-20180709150244-8b13a72661da // indirect
	github.com/gabriel-vasile/mimetype v1.4.3 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.22.0 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	golang.org/x/crypto v0.31.0 // indirect
	golang.org/x/sys v0.28.0 // indirect
	golang.org/x/text v0.21.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace h12.io/socks => github.com/h12w/socks v1.0.3
