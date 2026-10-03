package cells

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"heliosian/internal/id"
	"heliosian/internal/store"
)

const (
	dateFormat      = "2006-01-02"
	dateTimeFormat  = "2006-01-02 15:04"
	StampFormat     = "2006-01-02 15:04:05"
	maxURLLength    = 1000
	MaxPrettyLength = 40
)

func YesNo(cell string, blank bool) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(cell)) {
	case "yes":
		return true, nil
	case "no":
		return false, nil
	case "":
		return blank, nil
	}
	return false, fmt.Errorf("%q is not Yes, No, or blank", cell)
}

func YesNoBlank(cell string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(cell)) {
	case "yes":
		return "Yes", nil
	case "no":
		return "No", nil
	case "":
		return "", nil
	}
	return "", fmt.Errorf("%q is not Yes, No, or blank", cell)
}

func OrDefault(cell string, inherited bool) bool {
	if cell == "" {
		return inherited
	}
	return cell == "Yes"
}

func YesNoCell(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}

func URL(raw string, optional bool) error {
	if raw == "" && optional {
		return nil
	}
	if len(raw) > maxURLLength {
		return fmt.Errorf("url is too long")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("url %q must be an absolute http(s) url", raw)
	}
	return nil
}

func When(cell string) (time.Time, error) {
	for _, layout := range []string{StampFormat, dateTimeFormat, dateFormat} {
		if t, err := time.Parse(layout, cell); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("%q is not a date like 2026-09-24, 2026-09-24 16:00 or 2026-09-24 16:00:05", cell)
}

func Span(start, end string) error {
	var from time.Time
	if start != "" {
		t, err := When(start)
		if err != nil {
			return fmt.Errorf("start %w", err)
		}
		from = t
	}
	if end == "" {
		return nil
	}
	to, err := When(end)
	if err != nil {
		return fmt.Errorf("end %w", err)
	}
	if start == "" {
		return fmt.Errorf("has an end but no start")
	}
	if to.Before(from) {
		return fmt.Errorf("ends before it starts")
	}
	return nil
}

func Added(cell string) error {
	if cell == "" {
		return nil
	}
	if _, err := When(cell); err != nil {
		return fmt.Errorf("added %w", err)
	}
	return nil
}

func Title(kind, title string, max int) error {
	if strings.TrimSpace(title) == "" {
		return fmt.Errorf("%s has no title", kind)
	}
	if len(title) > max {
		return fmt.Errorf("%s title %q is too long", kind, title)
	}
	if title != strings.TrimSpace(title) {
		return fmt.Errorf("%s title %q has surrounding spaces", kind, title)
	}
	return nil
}

var prettyForm = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func NormalizePretty(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func CheckPretty(pretty string) error {
	if pretty == "" {
		return nil
	}
	if len(pretty) > MaxPrettyLength {
		return fmt.Errorf("pretty id %q is too long", pretty)
	}
	if !prettyForm.MatchString(pretty) {
		return fmt.Errorf("pretty id %q is not lower-case letters, digits and hyphens", pretty)
	}
	if _, ok := id.Parse(pretty); ok {
		return fmt.Errorf("pretty id %q reads as an id", pretty)
	}
	return nil
}

type Images interface {
	Has(key string) (bool, error)
}

func ImageURL(images Images, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", nil
	}
	found, err := images.Has(name)
	if err != nil {
		return "", fmt.Errorf("image %q: %w", name, err)
	}
	if !found {
		return "", fmt.Errorf("image %q does not exist", name)
	}
	return "/" + name, nil
}

func ImageNames(columns []string, tables ...[]store.Row) []string {
	names := []string{}
	for _, table := range tables {
		for _, row := range table {
			for _, column := range columns {
				if name := strings.TrimSpace(row[column]); name != "" {
					names = append(names, name)
				}
			}
		}
	}
	return names
}

func DisplayName(email string) string {
	local, _, _ := strings.Cut(email, "@")
	words := strings.FieldsFunc(local, func(r rune) bool { return r == '.' || r == '_' || r == '-' })
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

func SplitList(cell string) []string {
	out := []string{}
	for _, item := range strings.Split(cell, ",") {
		item = strings.TrimSpace(item)
		if item != "" && !slices.Contains(out, item) {
			out = append(out, item)
		}
	}
	return out
}

func JoinList(items []string) string {
	return strings.Join(items, ", ")
}
