package store

import (
	"fmt"
	"slices"
)

func ParseSettings(rows []Row, known, optional []string) (map[string]string, error) {
	values := map[string]string{}
	for _, row := range rows {
		key := row["Key"]
		if key == "" {
			return nil, fmt.Errorf("Settings row %v has no key", row)
		}
		if !slices.Contains(known, key) {
			return nil, fmt.Errorf("Settings has unknown key %q", key)
		}
		if _, dup := values[key]; dup {
			return nil, fmt.Errorf("Settings has duplicate key %q", key)
		}
		values[key] = row["Value"]
	}
	for _, key := range known {
		if values[key] == "" && !slices.Contains(optional, key) {
			return nil, fmt.Errorf("Settings is missing %q", key)
		}
	}
	return values, nil
}
