package tomarkdown

import "strings"

var trailers = []string{
	"You received this message because you are subscribed to",
	"To view this discussion on the web visit",
	"To unsubscribe from this group and stop receiving emails",
}

var notices = []string{
	"This message (including any attachments) may contain confidential information",
	"Please do not forward school emails without permission",
	"Please do not forward emails without permission",
}

func Trim(markdown string) string {
	blocks := strings.Split(markdown, "\n\n")
	for i, block := range blocks {
		if containsAny(block, trailers) {
			blocks = blocks[:i]
			break
		}
	}
	kept := []string{}
	for _, block := range blocks {
		if containsAny(block, notices) {
			continue
		}
		kept = append(kept, block)
	}
	for len(kept) > 0 {
		last := strings.TrimSpace(kept[len(kept)-1])
		if last != "" && last != "--" && last != "---" && last != "-" {
			break
		}
		kept = kept[:len(kept)-1]
	}
	return strings.TrimSpace(strings.Join(kept, "\n\n"))
}

func containsAny(block string, phrases []string) bool {
	for _, phrase := range phrases {
		if strings.Contains(block, phrase) {
			return true
		}
	}
	return false
}

func Text(text string) string {
	paragraphs := []string{}
	for _, block := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n\n") {
		lines := []string{}
		for _, line := range strings.Split(block, "\n") {
			if line = strings.TrimSpace(line); line != "" {
				lines = append(lines, line)
			}
		}
		if len(lines) == 0 {
			continue
		}
		joined := lines[0]
		for _, line := range lines[1:] {
			if strings.HasPrefix(line, "-") || strings.HasPrefix(line, "*") || strings.HasPrefix(line, ">") || strings.HasPrefix(line, "#") {
				joined += "\n" + line
				continue
			}
			joined += " " + line
		}
		paragraphs = append(paragraphs, joined)
	}
	return strings.Join(paragraphs, "\n\n")
}
