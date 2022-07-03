package internal

import (
	"crypto/md5"
	"encoding/hex"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func isLocalIP(host string) bool {
	ips, err := net.LookupIP(host)
	if err != nil {
		return false
	}
	for _, ip := range ips {
		if ip.IsLoopback() {
			return true
		}
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	for _, addr := range addrs {
		_, ipNet, err := net.ParseCIDR(addr.String())
		if err == nil {
			for _, ip := range ips {
				if ipNet.Contains(ip) {
					return true
				}
			}
		}
	}
	return false
}

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
			break
		case "https":
		case "wss":
			port = 443
			break
		default:
			break
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

var htmlTagReg = regexp.MustCompile(`>\s+<`)

// ReduceHTMLSpace 去除html tag 间的空格字符
func ReduceHTMLSpace(html string) string {
	return htmlTagReg.ReplaceAllString(html, "><")
}

func SetInterval(call func(), dur time.Duration) *time.Ticker {
	ticker := time.NewTicker(dur)
	go func() {
		for {
			select {
			case <-ticker.C:
				call()
			}
		}
	}()
	return ticker
}

func StrMd5(str string) string {
	h := md5.New()
	h.Write([]byte(str))
	return hex.EncodeToString(h.Sum(nil))
}
