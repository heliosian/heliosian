package main

import (
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

const lineBreak = "\x00"

var blockTags = map[string]bool{
	"p": true, "div": true, "li": true, "blockquote": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
}

func flattenBio(fragment string) (string, error) {
	parent := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
	nodes, err := html.ParseFragment(strings.NewReader(fragment), parent)
	if err != nil {
		return "", err
	}
	text := &strings.Builder{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			text.WriteString(n.Data)
		}
		if n.Type == html.ElementNode && n.Data == "br" {
			text.WriteString(lineBreak)
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
		if n.Type == html.ElementNode && blockTags[n.Data] {
			text.WriteString(lineBreak + lineBreak)
		}
	}
	for _, n := range nodes {
		walk(n)
	}
	lines := strings.Split(text.String(), lineBreak)
	for i, line := range lines {
		lines[i] = strings.Join(strings.Fields(line), " ")
	}
	flat := strings.Join(lines, "\n")
	for strings.Contains(flat, "\n\n\n") {
		flat = strings.ReplaceAll(flat, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(flat), nil
}
