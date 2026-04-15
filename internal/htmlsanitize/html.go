//  Copyright(C) 2026 github.com/hidu  All Rights Reserved.
//  Author: hidu <duv123+git@gmail.com>
//  Date: 2026-04-15

package htmlsanitize

import (
	"bytes"
	"io"
	"strings"

	"golang.org/x/net/html"
)

func CleanReader(rd io.Reader) ([]byte, error) {
	doc, err := html.Parse(rd)
	if err != nil {
		return nil, err
	}

	sanitize(doc)
	return render(doc)
}

func CleanHTML(bf []byte) ([]byte, error) {
	return CleanReader(bytes.NewReader(bf))
}

// 递归清理 DOM
func sanitize(n *html.Node) {
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		if isRemoveNode(c) {
			n.RemoveChild(c)
		} else {
			cleanAttrs(c)
			sanitize(c)

			if isEmptyNode(c) {
				n.RemoveChild(c)
			}
		}
		c = next
	}
}

// render HTML
func render(n *html.Node) ([]byte, error) {
	var buf bytes.Buffer
	if err := html.Render(&buf, n); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func isRemoveNode(n *html.Node) bool {
	if n.Type == html.CommentNode {
		return true
	}
	if n.Type != html.ElementNode {
		return false
	}

	tag := strings.ToLower(n.Data)

	switch tag {
	case "script", "style", "iframe":
		return true
	case "link":
		for _, a := range n.Attr {
			if strings.ToLower(a.Key) == "rel" &&
				strings.Contains(strings.ToLower(a.Val), "stylesheet") {
				return true
			}
		}
	}
	return false
}

// 判断是否是事件属性：onclick / onload / onxxx
func isEventAttr(key string) bool {
	k := strings.ToLower(key)
	return strings.HasPrefix(k, "on")
}

// 删除 style + event 属性
func cleanAttrs(n *html.Node) {
	n.Data = strings.TrimSpace(n.Data)
	if n.Type != html.ElementNode {
		return
	}
	tag := strings.ToLower(n.Data)
	newAttrs := make([]html.Attribute, 0, len(n.Attr))
	for _, a := range n.Attr {
		// 属性值为空的，直接删除属性
		if a.Val == "" {
			continue
		}

		k := strings.ToLower(a.Key)
		switch k {
		case "style", "class", "id":
			continue
		}

		if isEventAttr(k) {
			continue
		}
		// 删除 a 标签的  target 属性
		if tag == "a" && k == "target" {
			continue
		}

		if tag == "a" && k == "href" && (a.Val == "javascript:;" || a.Val == "javascript:void(0)") {
			continue
		}
		newAttrs = append(newAttrs, a)
	}
	n.Attr = newAttrs
}

func isEmptyNode(n *html.Node) bool {
	if n.Type != html.ElementNode {
		return false
	}

	// 有属性 → 不算空
	if len(n.Attr) > 0 {
		return false
	}

	// 有子节点 → 检查是否都为空
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		switch c.Type {
		case html.TextNode:
			if strings.TrimSpace(c.Data) != "" {
				return false
			}
		default:
			// 还有子元素 → 不算空
			return false
		}
	}

	return true
}
