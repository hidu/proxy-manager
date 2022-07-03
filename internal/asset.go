// Copyright(C) 2022 github.com/fsgo  All Rights Reserved.
// Author: hidu <duv123@gmail.com>
// Date: 2022/7/1

package internal

import (
	"embed"
)

//go:embed asset/*
var files embed.FS

func AssetGetContent(fp string) []byte {
	content, _ := files.ReadFile("asset/" + fp)
	return content
}
