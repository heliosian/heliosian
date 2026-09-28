package id

import (
	"fmt"
	"strings"
)

const (
	AliasesTab  = "Aliases"
	AliasColumn = "Alias"
	IDColumn    = "ID"
)

var AliasColumns = []string{AliasColumn, IDColumn}

type Aliases map[string]string

func ParseAliases(rows []map[string]string) (Aliases, error) {
	out := Aliases{}
	for _, row := range rows {
		alias := strings.ToLower(strings.TrimSpace(row[AliasColumn]))
		target, ok := Parse(row[IDColumn])
		if alias == "" || !ok {
			return nil, fmt.Errorf("aliases row %v needs an alias and an ID", row)
		}
		if _, dup := out[alias]; dup {
			return nil, fmt.Errorf("alias %q is listed twice", alias)
		}
		out[alias] = target
	}
	return out, nil
}

func (a Aliases) Resolve(s string) string {
	if target, ok := a[strings.ToLower(strings.TrimSpace(s))]; ok {
		return target
	}
	return s
}
