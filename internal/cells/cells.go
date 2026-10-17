package cells

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"heliosian/internal/id"
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
