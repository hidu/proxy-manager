package main

import (
	"flag"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
)

var proxy = flag.String("proxy", "http://127.0.0.1:8090", "proxy info")
var target = flag.String("url", "http://www.baidu.com", "url get")
var dumpBody = flag.Bool("body", false, "dump the response body")

func main() {
	flag.Parse()
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: func(req *http.Request) (*url.URL, error) {
				return url.Parse(*proxy)
			},
		},
	}
	resp, err := client.Get(*target)
	fmt.Println("err:", err)
	dump, _ := httputil.DumpResponse(resp, *dumpBody)
	fmt.Println(string(dump))
}
