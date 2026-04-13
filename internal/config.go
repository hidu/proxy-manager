package internal

import (
	"log"
	"maps"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"

	"github.com/xanygo/anygo/xattr"
)

func getAliveCheckURL() string {
	str, _ := xattr.GetAs[string]("AliveCheckURL")
	if str != "" && strings.Contains(str, "{rand}") {
		str = strings.ReplaceAll(str, "{rand}", strconv.Itoa(rand.Int()))
	}
	str = strings.TrimSpace(str)
	if str != "" {
		return str
	}
	return "https://ifconfig.me/ip"
}

func getCheckInterval() time.Duration {
	val := xattr.GetDefault[time.Duration]("CheckInterval", 0)
	if val > 0 {
		return val * time.Second
	}
	return 300 * time.Second
}

func getProxyTimeout() time.Duration {
	num := xattr.GetDefault[time.Duration]("ProxyTimeout", 0)
	if num > 0 {
		return num * time.Second
	}
	return 30 * time.Second
}

// 使用代理时候的，默认重试次数
func getProxyRetry() int {
	num := xattr.GetDefault[int]("ProxyRetry", 0)
	if num > 0 {
		return num
	}
	return 2
}

// 使用代理时候的，最大重试次数
// 客户端通过 HTTP Header [X-Man-Retry] 指定 ProxyReTry
func getProxyRetryMax() int {
	num := xattr.GetDefault[int]("ProxyRetryMax", 0)
	if num > 0 {
		return num
	}
	return 10
}

func parserProxiesFromTxt(txt string) *ProxyList {
	defaultValues := make(map[string]string)
	defaultValues["proxy"] = "required"
	defaultValues["weight"] = "1"
	lines := strings.Split(txt, "\n")

	pl := newProxyList(nil)
	for _, line := range lines {
		if b, _, ok := strings.Cut(line, "#"); ok {
			line = b
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		val := maps.Clone(defaultValues)
		for _, field := range fields {
			kv := strings.SplitN(field, "=", 2)
			if len(kv) == 2 {
				val[kv[0]] = kv[1]
			}
		}

		if p := parseProxyLine(val); p != nil {
			pl.Add(p)
		}
	}
	return pl
}

func parseProxyLine(info map[string]string) *proxyEntry {
	if info == nil {
		return nil
	}
	p := newProxy(info["proxy"])
	if p == nil {
		return nil
	}
	intValues := make(map[string]int64)
	intFields := []string{"weight"}
	var err error
	for _, fieldName := range intFields {
		intValues[fieldName], err = strconv.ParseInt(info[fieldName], 10, 60)
		if err != nil {
			log.Println("parse [", fieldName, "] failed, not int. err:", err)
			intValues[fieldName] = 0
		}
	}
	p.Base.Weight = int(intValues["weight"])
	return p
}
