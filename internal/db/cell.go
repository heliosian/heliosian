package db

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"heliosian/internal/cells"
	"heliosian/internal/store"
)

var (
	wholeNumber = regexp.MustCompile(`^[0-9]+$`)
	money       = regexp.MustCompile(`^[0-9]+(\.[0-9]{1,2})?$`)
	address     = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
)

func (c Column) Check(raw string) error {
	if c.Generated {
		return fmt.Errorf("%s is generated, never stored", c.Name)
	}
	if strings.TrimSpace(raw) == "" {
		if c.Required {
			return fmt.Errorf("%s is required", c.Name)
		}
		return nil
	}
	if err := c.check(raw); err != nil {
		return fmt.Errorf("%s: %w", c.Name, err)
	}
	return nil
}

func (c Column) check(raw string) error {
	v := strings.TrimSpace(raw)
	switch c.Kind {
	case Text:
		return nil
	case Enum:
		names := c.ValueNames()
		for _, value := range names {
			if strings.EqualFold(v, value) {
				return nil
			}
		}
		return fmt.Errorf("%q is not one of %s", raw, strings.Join(names, ", "))
	case ID:
		prefix, ok := ParseID(raw)
		if !ok || prefix != c.Prefix {
			return fmt.Errorf("%q is not a %s id", raw, c.Prefix)
		}
		return nil
	case Ref:
		return c.reference(raw)
	case Refs:
		for _, item := range strings.Split(raw, ",") {
			if err := c.reference(strings.TrimSpace(item)); err != nil {
				return err
			}
		}
		return nil
	case Bool:
		_, err := cells.YesNo(v, false)
		return err
	case Int:
		if !wholeNumber.MatchString(v) {
			return fmt.Errorf("%q is not a whole number", raw)
		}
		return nil
	case Money:
		if !money.MatchString(v) {
			return fmt.Errorf("%q is not an amount like 12.50", raw)
		}
		return nil
	case Float:
		_, err := strconv.ParseFloat(v, 64)
		return err
	case Date:
		if _, err := time.Parse(time.DateOnly, v); err != nil {
			return fmt.Errorf("%q is not a date like 2026-09-24", raw)
		}
		return nil
	case Moment:
		_, err := cells.When(v)
		return err
	case Blob:
		if v != raw {
			return fmt.Errorf("object name %q has surrounding spaces", raw)
		}
		return nil
	case Order:
		return store.CheckKey(v)
	case Email:
		if !address.MatchString(v) {
			return fmt.Errorf("%q is not an email address", raw)
		}
		return nil
	case URL:
		return cells.URL(v, false)
	}
	return fmt.Errorf("column kind %d has no check", c.Kind)
}

func (c Column) reference(raw string) error {
	table, ok := TableOf(raw)
	if !ok {
		return fmt.Errorf("%q is not an id", raw)
	}
	if c.Target != "" && table != c.Target {
		return fmt.Errorf("%q is a %s id, not a %s id", raw, table, c.Target)
	}
	return nil
}

func (t Table) Check(row map[string]string) error {
	for _, c := range t.Columns {
		if c.Generated {
			continue
		}
		if err := c.Check(row[c.Name]); err != nil {
			return fmt.Errorf("%s: %w", t.Name, err)
		}
	}
	return nil
}
