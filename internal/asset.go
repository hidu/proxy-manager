// Copyright(C) 2022 github.com/fsgo  All Rights Reserved.
// Author: hidu <duv123@gmail.com>
// Date: 2022/7/1

package internal

import (
	"embed"
	"html/template"

	"github.com/xanygo/anygo/xhtml"
)

//go:embed asset/*
var files embed.FS

func AssetGetContent(fp string) []byte {
	content, _ := files.ReadFile("asset/" + fp)
	return content
}

var tpl *template.Template

func init() {
	tpl = template.Must(template.New("layout").Funcs(xhtml.FuncMap).ParseFS(files, "asset/tpl/*"))
}
