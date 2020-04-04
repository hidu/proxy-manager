package internal

import (
	"log"
	"strconv"
)

func getInt64(str string) int64 {
	if str == "" {
		return 0
	}
	v, err := strconv.ParseInt(str, 10, 64)
	if err != nil {
		log.Println("parse2int failed,[", str, "]", err)
		return 0
	}
	return v
}
