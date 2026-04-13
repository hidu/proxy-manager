package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"time"
)

var proxy = flag.String("proxy", "http://127.0.0.1:8128", "proxy info")
var target = flag.String("url", "https://ifconfig.me/all.json", "url get")
var status_ok = flag.String("status_ok", "200,304", "x-man-status-ok")
var retry = flag.Int("retry", -1, "X-Man-ReTry")

func main() {
	flag.Parse()
	proxyURL, err := url.Parse(*proxy)
	if err != nil {
		log.Fatalf("parser proxy %q: %v\n", *proxy, err)
	}
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			Proxy: func(req *http.Request) (*url.URL, error) {
				log.Println("using proxy", proxyURL.String())
				return proxyURL, nil
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
	if err != nil {
		log.Fatalln("fetch failed:", err.Error())
	}
	dump, _ := httputil.DumpResponse(resp, true)
	fmt.Println(string(dump))
}
