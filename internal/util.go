package internal

import (
	"crypto/md5"
	"encoding/hex"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func getHostPortFromReq(req *http.Request) (host string, port int, err error) {
	var urlStr string
	if req.URL.Scheme != "" {
		urlStr = req.URL.Scheme + "://" + req.Host
	} else {
		urlStr = "http://" + req.Host
	}
	return getHostPortFromURL(urlStr)
}

func getHostPortFromURL(urlStr string) (host string, port int, err error) {
	urlObj, err := url.Parse(urlStr)
	if err != nil {
		return "", 0, err
	}
	host, port, err = parseHostPort(urlObj.Host)
	if err == nil && port == 0 {
		switch urlObj.Scheme {
		case "http":
		case "ws":
			port = 80
		case "https":
		case "wss":
			port = 443
		default:
		}
	}
	return
}

func parseHostPort(hostPortStr string) (host string, port int, err error) {
	if !strings.Contains(hostPortStr, ":") {
		hostPortStr += ":0"
	}
	var portStr string
	host, portStr, err = net.SplitHostPort(hostPortStr)
	if err != nil {
		return "", 0, err
	}
	port, err = strconv.Atoi(portStr)
	return host, port, err
}

func SetInterval(call func(), dur time.Duration) *time.Ticker {
	ticker := time.NewTicker(dur)
	go func() {
		for range ticker.C {
			call()
		}
	}()
	return ticker
}

func StrMd5(str string) string {
	h := md5.New()
	h.Write([]byte(str))
	return hex.EncodeToString(h.Sum(nil))
}
