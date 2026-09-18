package artifacts

import (
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

var skipped = map[string]bool{"script": true, "style": true, "head": true, "title": true, "noscript": true, "template": true, "img": true, "svg": true, "iframe": true}

var blocks = map[string]bool{
	"p": true, "div": true, "ul": true, "ol": true, "table": true, "thead": true, "tbody": true, "tfoot": true, "tr": true,
	"section": true, "article": true, "header": true, "footer": true, "main": true, "nav": true, "aside": true,
	"blockquote": true, "center": true, "pre": true, "address": true, "figure": true, "figcaption": true, "dl": true, "dt": true, "dd": true, "form": true,
}

var headings = map[string]int{"h1": 1, "h2": 2, "h3": 3, "h4": 4, "h5": 5, "h6": 6}

type renderer struct {
	resolve func(string) string
	blocks  []string
	line    strings.Builder
	prefix  string
}

// Hrefs is every address the page links to, for a caller that wants to
// resolve them all before the page is rendered.
func Hrefs(source string) []string {
	doc, err := html.Parse(strings.NewReader(source))
	if err != nil {
		return nil
	}
	out := []string{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, attr := range n.Attr {
				if attr.Key == "href" {
					out = append(out, strings.TrimSpace(attr.Val))
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return out
}

func Markdown(source string, resolve func(string) string) (string, error) {
	doc, err := html.Parse(strings.NewReader(source))
	if err != nil {
		return "", err
	}
	r := &renderer{resolve: resolve}
	r.walk(doc)
	r.flush()
	out := &strings.Builder{}
	for i, block := range r.blocks {
		if i > 0 {
			if strings.HasPrefix(r.blocks[i-1], "- ") && strings.HasPrefix(block, "- ") {
				out.WriteString("\n")
			} else {
				out.WriteString("\n\n")
			}
		}
		out.WriteString(block)
	}
	return out.String(), nil
}

func (r *renderer) walk(n *html.Node) {
	switch n.Type {
	case html.TextNode:
		r.text(n.Data)
		return
	case html.DocumentNode:
		r.children(n)
		return
	case html.ElementNode:
	default:
		return
	}
	if skipped[n.Data] {
		return
	}
	if level, ok := headings[n.Data]; ok {
		r.flush()
		r.prefix = strings.Repeat("#", level) + " "
		r.children(n)
		r.flush()
		return
	}
	switch n.Data {
	case "br":
		r.line.WriteString("\n")
	case "hr":
		r.flush()
		r.blocks = append(r.blocks, "---")
	case "li":
		r.flush()
		r.prefix = "- "
		r.children(n)
		r.flush()
	case "td", "th":
		if s := r.line.String(); s != "" && !strings.HasSuffix(s, "\n") && !strings.HasSuffix(s, " | ") {
			r.line.WriteString(" | ")
		}
		r.children(n)
	case "a":
		r.link(n)
	case "strong", "b":
		r.wrap(n, "**")
	case "em", "i":
		r.wrap(n, "*")
	default:
		if blocks[n.Data] {
			r.flush()
			r.children(n)
			r.flush()
			return
		}
		r.children(n)
	}
}

func (r *renderer) children(n *html.Node) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		r.walk(c)
	}
}

func (r *renderer) text(s string) {
	s = strings.ReplaceAll(s, " ", " ")
	leading := s != "" && strings.TrimLeft(s, " \t\r\n") != s
	trailing := s != "" && strings.TrimRight(s, " \t\r\n") != s
	collapsed := strings.Join(strings.Fields(s), " ")
	if collapsed == "" {
		if leading || trailing {
			r.space()
		}
		return
	}
	if leading {
		r.space()
	}
	r.line.WriteString(collapsed)
	if trailing {
		r.line.WriteString(" ")
	}
}

func (r *renderer) space() {
	s := r.line.String()
	if s == "" || strings.HasSuffix(s, " ") || strings.HasSuffix(s, "\n") {
		return
	}
	r.line.WriteString(" ")
}

// capture renders a node's children and hands back what they wrote on the
// line so far, for an inline element to wrap. A link or a bold that turns
// out to hold whole blocks - mail templates put links around tables - has
// written those blocks already and has nothing left to wrap, which inline
// reports.
func (r *renderer) capture(n *html.Node) (core string, leading, trailing, inline bool) {
	blocks := len(r.blocks)
	before := r.line.String()
	r.children(n)
	current := r.line.String()
	if len(r.blocks) != blocks || !strings.HasPrefix(current, before) {
		return "", false, false, false
	}
	inner := current[len(before):]
	r.line.Reset()
	r.line.WriteString(before)
	core = strings.TrimSpace(inner)
	leading = inner != "" && strings.TrimLeft(inner, " \n") != inner
	trailing = inner != "" && strings.TrimRight(inner, " \n") != inner
	return core, leading, trailing, true
}

func (r *renderer) wrap(n *html.Node, marker string) {
	core, leading, trailing, inline := r.capture(n)
	if !inline {
		return
	}
	if core == "" {
		if leading || trailing {
			r.space()
		}
		return
	}
	if leading {
		r.space()
	}
	r.line.WriteString(marker + core + marker)
	if trailing {
		r.line.WriteString(" ")
	}
}

func (r *renderer) link(n *html.Node) {
	href := ""
	for _, attr := range n.Attr {
		if attr.Key == "href" {
			href = strings.TrimSpace(attr.Val)
		}
	}
	core, leading, trailing, inline := r.capture(n)
	if !inline {
		return
	}
	if core == "" {
		if leading || trailing {
			r.space()
		}
		return
	}
	if leading {
		r.space()
	}
	lower := strings.ToLower(href)
	target := ""
	if href != "" && !strings.HasPrefix(lower, "#") && !strings.HasPrefix(lower, "javascript:") {
		target = r.resolve(href)
	}
	// A link whose address is a redirect nobody can follow any more keeps
	// its words; an address that would only identify the reader is no use.
	if target == "" {
		r.line.WriteString(core)
	} else {
		r.line.WriteString("[" + core + "](" + target + ")")
	}
	if trailing {
		r.line.WriteString(" ")
	}
}

func (r *renderer) flush() {
	prefix := r.prefix
	r.prefix = ""
	raw := r.line.String()
	r.line.Reset()
	lines := []string{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(line), "|"))
		line = strings.TrimSpace(strings.TrimPrefix(line, "|"))
		if line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return
	}
	// A mail template sets its section banners in bold capitals rather than
	// as headings, so a whole block that is nothing else is one.
	if prefix == "" && len(lines) == 1 {
		if banner, ok := sectionBanner(lines[0]); ok {
			r.blocks = append(r.blocks, "# "+banner)
			return
		}
	}
	indent := strings.Repeat(" ", len(prefix))
	for i := range lines {
		if i == 0 {
			lines[i] = prefix + lines[i]
		} else {
			lines[i] = indent + lines[i]
		}
	}
	r.blocks = append(r.blocks, strings.Join(lines, "\n"))
}

// sectionBanner is the words of a line that is one piece of bold text in
// capitals and nothing else, which a template uses where a heading belongs.
func sectionBanner(line string) (string, bool) {
	inner, ok := strings.CutPrefix(line, "**")
	if !ok {
		return "", false
	}
	inner, ok = strings.CutSuffix(inner, "**")
	if !ok || strings.Contains(inner, "**") || len(inner) > 80 {
		return "", false
	}
	// The banner is sometimes a link to the thing it names.
	words := linkText.ReplaceAllString(inner, "$1")
	if !capitals(words) {
		return "", false
	}
	return strings.TrimSpace(words), true
}

var linkText = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
