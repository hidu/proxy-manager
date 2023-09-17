package main

import (
	"flag"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
)

var proxy = flag.String("proxy", "http://127.0.0.1:8128", "proxy info")
var target = flag.String("url", "http://www.baidu.com", "url get")
var dumpBody = flag.Bool("body", false, "dump the response body")
var status_ok = flag.String("status_ok", "200,304", "x-man-status-ok")
var retry = flag.Int("retry", -1, "X-Man-ReTry")

func main() {
	flag.Parse()
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: func(req *http.Request) (*url.URL, error) {
				return url.Parse(*proxy)
			},
		},
	}
	req, _ := http.NewRequest("GET", *target, nil)
	if *status_ok != "" {
		req.Header.Set("X-Man-Status-Ok", *status_ok)
	}
	if *retry > -1 {
		req.Header.Set("X-Man-Retry", strconv.Itoa(*retry))
	}
	resp, err := client.Do(req)
	fmt.Println("err:", err)
	dump, _ := httputil.DumpResponse(resp, *dumpBody)
	fmt.Println(string(dump))
}
