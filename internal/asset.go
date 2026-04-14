// Copyright(C) 2022 github.com/fsgo  All Rights Reserved.
// Author: hidu <duv123@gmail.com>
// Date: 2022/7/1

package internal

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"

	"github.com/xanygo/anygo/xhtml"
)

//go:embed asset/*
var files embed.FS

var tpl *template.Template

func init() {
	tpl = template.Must(template.New("layout").Funcs(xhtml.FuncMap).Funcs(template.FuncMap{
		"my_num": func(num any) string {
			str := fmt.Sprint(num)
			if str == "0" {
				return ""
			}
			return str
		},
	}).ParseFS(files, "asset/tpl/*"))
}

func renderHTML(fileName string, values map[string]any, layout bool) []byte {
	w := &bytes.Buffer{}
	err := tpl.ExecuteTemplate(w, fileName, values)
	if err != nil {
		w.WriteString("reader error:" + err.Error())
	}
	if !layout {
		return w.Bytes()
	}
	values["body"] = w.String()
	return renderHTML("layout.html", values, false)
}
