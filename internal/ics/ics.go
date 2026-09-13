// Package ics parses iCalendar feeds into concrete event instances.
package ics

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
)

const maxInstances = 5000

// End is exclusive: an all-day event on one day ends at the next midnight, as the
// feed states it. Key is the UID, or UID/instance for one occurrence of a repeat.
type Event struct {
	Key         string
	UID         string
	Start       time.Time
	End         time.Time
	AllDay      bool
	Summary     string
	Location    string
	Description string
	Modified    time.Time
	Sequence    int
	Status      string
	Recurring   bool
}

type property struct {
	name   string
	params map[string]string
	value  string
}

type component struct {
	props []property
}

func (c component) first(name string) (property, bool) {
	for _, p := range c.props {
		if p.name == name {
			return p, true
		}
	}
	return property{}, false
}

func (c component) all(name string) []property {
	out := []property{}
	for _, p := range c.props {
		if p.name == name {
			out = append(out, p)
		}
	}
	return out
}

func unfold(r io.Reader) ([]string, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	lines := []string{}
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) && len(lines) > 0 {
			lines[len(lines)-1] += line[1:]
			continue
		}
		lines = append(lines, line)
	}
	return lines, scanner.Err()
}

func parseProperty(line string) (property, error) {
	p := property{params: map[string]string{}}
	i := 0
	for i < len(line) && line[i] != ';' && line[i] != ':' {
		i++
	}
	if i == len(line) {
		return p, fmt.Errorf("property has no value: %q", line)
	}
	p.name = strings.ToUpper(line[:i])
	for line[i] == ';' {
		i++
		j := i
		for j < len(line) && line[j] != '=' {
			j++
		}
		if j == len(line) {
			return p, fmt.Errorf("parameter has no value: %q", line)
		}
		key := strings.ToUpper(line[i:j])
		i = j + 1
		value := &strings.Builder{}
		for i < len(line) && line[i] != ';' && line[i] != ':' {
			if line[i] == '"' {
				i++
				for i < len(line) && line[i] != '"' {
					value.WriteByte(line[i])
					i++
				}
				if i < len(line) {
					i++
				}
				continue
			}
			value.WriteByte(line[i])
			i++
		}
		if i == len(line) {
			return p, fmt.Errorf("property has no value: %q", line)
		}
		p.params[key] = value.String()
	}
	p.value = line[i+1:]
	return p, nil
}

func unescape(s string) string {
	out := &strings.Builder{}
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 == len(s) {
			out.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 'n', 'N':
			out.WriteByte('\n')
		default:
			out.WriteByte(s[i])
		}
	}
	return out.String()
}

func parseComponents(lines []string) ([]component, error) {
	events := []component{}
	var current *component
	depth := 0
	for _, line := range lines {
		if line == "" {
			continue
		}
		p, err := parseProperty(line)
		if err != nil {
			return nil, err
		}
		switch {
		case p.name == "BEGIN" && strings.EqualFold(p.value, "VEVENT"):
			current = &component{}
			depth = 1
		case p.name == "END" && strings.EqualFold(p.value, "VEVENT"):
			if current != nil {
				events = append(events, *current)
			}
			current = nil
			depth = 0
		case p.name == "BEGIN":
			depth++
		case p.name == "END":
			depth--
		case current != nil && depth == 1:
			current.props = append(current.props, p)
		}
	}
	return events, nil
}

func parseTime(p property, loc *time.Location) (time.Time, bool, error) {
	v := p.value
	if p.params["VALUE"] == "DATE" || len(v) == 8 {
		t, err := time.ParseInLocation("20060102", v, loc)
		return t, true, err
	}
	if strings.HasSuffix(v, "Z") {
		t, err := time.Parse("20060102T150405Z", v)
		return t.In(loc), false, err
	}
	zone := loc
	if tzid := p.params["TZID"]; tzid != "" {
		z, err := time.LoadLocation(tzid)
		if err != nil {
			return time.Time{}, false, fmt.Errorf("time zone %q: %w", tzid, err)
		}
		zone = z
	}
	t, err := time.ParseInLocation("20060102T150405", v, zone)
	return t.In(loc), false, err
}

type weekdayRule struct {
	n   int
	day time.Weekday
}

type rule struct {
	freq     string
	interval int
	count    int
	until    time.Time
	hasUntil bool
	byDay    []weekdayRule
	wkst     time.Weekday
}

var weekdays = map[string]time.Weekday{
	"SU": time.Sunday, "MO": time.Monday, "TU": time.Tuesday, "WE": time.Wednesday,
	"TH": time.Thursday, "FR": time.Friday, "SA": time.Saturday,
}

func parseRule(value string, loc *time.Location) (rule, error) {
	r := rule{interval: 1, wkst: time.Monday}
	for _, part := range strings.Split(value, ";") {
		key, val, _ := strings.Cut(part, "=")
		switch strings.ToUpper(key) {
		case "FREQ":
			r.freq = strings.ToUpper(val)
		case "INTERVAL":
			n, err := strconv.Atoi(val)
			if err != nil || n < 1 {
				return r, fmt.Errorf("bad interval %q", val)
			}
			r.interval = n
		case "COUNT":
			n, err := strconv.Atoi(val)
			if err != nil || n < 1 {
				return r, fmt.Errorf("bad count %q", val)
			}
			r.count = n
		case "UNTIL":
			t, allDay, err := parseTime(property{value: val, params: map[string]string{}}, loc)
			if err != nil {
				return r, fmt.Errorf("bad until %q", val)
			}
			if allDay {
				t = t.AddDate(0, 0, 1)
			} else {
				t = t.Add(time.Second)
			}
			r.until, r.hasUntil = t, true
		case "BYDAY":
			for _, item := range strings.Split(val, ",") {
				item = strings.ToUpper(item)
				n := 0
				if len(item) > 2 {
					parsed, err := strconv.Atoi(item[:len(item)-2])
					if err != nil {
						return r, fmt.Errorf("bad byday %q", item)
					}
					n = parsed
					item = item[len(item)-2:]
				}
				day, ok := weekdays[item]
				if !ok {
					return r, fmt.Errorf("bad byday %q", item)
				}
				r.byDay = append(r.byDay, weekdayRule{n: n, day: day})
			}
		case "WKST":
			day, ok := weekdays[strings.ToUpper(val)]
			if !ok {
				return r, fmt.Errorf("bad wkst %q", val)
			}
			r.wkst = day
		}
	}
	switch r.freq {
	case "DAILY", "WEEKLY", "MONTHLY", "YEARLY":
	default:
		return r, fmt.Errorf("unsupported frequency %q", r.freq)
	}
	return r, nil
}

func at(day time.Time, clock time.Time) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day(), clock.Hour(), clock.Minute(), clock.Second(), 0, clock.Location())
}

func nthWeekday(year int, month time.Month, w weekdayRule, clock time.Time) (time.Time, bool) {
	if w.n >= 0 {
		first := time.Date(year, month, 1, 0, 0, 0, 0, clock.Location())
		offset := (int(w.day) - int(first.Weekday()) + 7) % 7
		n := max(w.n, 1)
		day := first.AddDate(0, 0, offset+(n-1)*7)
		return at(day, clock), day.Month() == month
	}
	last := time.Date(year, month+1, 0, 0, 0, 0, 0, clock.Location())
	offset := (int(last.Weekday()) - int(w.day) + 7) % 7
	day := last.AddDate(0, 0, -offset+(w.n+1)*7)
	return at(day, clock), day.Month() == month
}

// instances lists the rule's occurrences from start, in order, up to the
// rule's own end or to, whichever comes first.
func (r rule) instances(start, to time.Time) []time.Time {
	out := []time.Time{}
	stop := func(t time.Time) bool {
		if !t.Before(to) {
			return true
		}
		if r.hasUntil && !t.Before(r.until) {
			return true
		}
		return r.count > 0 && len(out) >= r.count
	}
	emit := func(t time.Time) bool {
		if t.Before(start) {
			return false
		}
		if stop(t) {
			return true
		}
		out = append(out, t)
		return len(out) >= maxInstances
	}
	switch r.freq {
	case "DAILY":
		for i := 0; ; i++ {
			if emit(start.AddDate(0, 0, i*r.interval)) {
				return out
			}
		}
	case "WEEKLY":
		if len(r.byDay) == 0 {
			for i := 0; ; i++ {
				if emit(start.AddDate(0, 0, 7*i*r.interval)) {
					return out
				}
			}
		}
		offsets := []int{}
		for _, w := range r.byDay {
			offsets = append(offsets, (int(w.day)-int(r.wkst)+7)%7)
		}
		sort.Ints(offsets)
		weekStart := start.AddDate(0, 0, -((int(start.Weekday()) - int(r.wkst) + 7) % 7))
		for week := 0; ; week++ {
			base := weekStart.AddDate(0, 0, 7*week*r.interval)
			for _, offset := range offsets {
				if emit(base.AddDate(0, 0, offset)) {
					return out
				}
			}
		}
	case "MONTHLY":
		for i := 0; ; i++ {
			first := time.Date(start.Year(), start.Month()+time.Month(i*r.interval), 1, 0, 0, 0, 0, start.Location())
			if len(r.byDay) == 0 {
				day := time.Date(first.Year(), first.Month(), start.Day(), 0, 0, 0, 0, start.Location())
				if day.Month() != first.Month() {
					continue
				}
				if emit(at(day, start)) {
					return out
				}
				continue
			}
			candidates := []time.Time{}
			for _, w := range r.byDay {
				if t, ok := nthWeekday(first.Year(), first.Month(), w, start); ok {
					candidates = append(candidates, t)
				}
			}
			sort.Slice(candidates, func(a, b int) bool { return candidates[a].Before(candidates[b]) })
			for _, t := range candidates {
				if emit(t) {
					return out
				}
			}
			if len(candidates) == 0 && first.After(to) {
				return out
			}
		}
	case "YEARLY":
		for i := 0; ; i++ {
			t := start.AddDate(i*r.interval, 0, 0)
			if t.Month() != start.Month() {
				continue
			}
			if emit(t) {
				return out
			}
		}
	}
	return out
}

func instanceKey(uid string, t time.Time, allDay bool) string {
	if allDay {
		return uid + "/" + t.Format("20060102")
	}
	return uid + "/" + t.Format("20060102T150405")
}

func build(c component, loc *time.Location) (Event, error) {
	e := Event{}
	uid, ok := c.first("UID")
	if !ok {
		return e, fmt.Errorf("event has no uid")
	}
	e.UID = uid.value
	e.Key = uid.value
	startProp, ok := c.first("DTSTART")
	if !ok {
		return e, fmt.Errorf("event %s has no start", e.UID)
	}
	start, allDay, err := parseTime(startProp, loc)
	if err != nil {
		return e, fmt.Errorf("event %s start: %w", e.UID, err)
	}
	e.Start, e.AllDay = start, allDay
	if endProp, ok := c.first("DTEND"); ok {
		end, _, err := parseTime(endProp, loc)
		if err != nil {
			return e, fmt.Errorf("event %s end: %w", e.UID, err)
		}
		e.End = end
	} else if allDay {
		e.End = start.AddDate(0, 0, 1)
	} else {
		e.End = start
	}
	if p, ok := c.first("SUMMARY"); ok {
		e.Summary = unescape(p.value)
	}
	if p, ok := c.first("LOCATION"); ok {
		e.Location = unescape(p.value)
	}
	if p, ok := c.first("DESCRIPTION"); ok {
		e.Description = unescape(p.value)
	}
	if p, ok := c.first("STATUS"); ok {
		e.Status = strings.ToUpper(p.value)
	}
	if p, ok := c.first("LAST-MODIFIED"); ok {
		t, _, err := parseTime(p, loc)
		if err != nil {
			return e, fmt.Errorf("event %s last-modified: %w", e.UID, err)
		}
		e.Modified = t
	}
	if p, ok := c.first("SEQUENCE"); ok {
		n, err := strconv.Atoi(p.value)
		if err != nil {
			return e, fmt.Errorf("event %s sequence: %w", e.UID, err)
		}
		e.Sequence = n
	}
	return e, nil
}

// Parse reads a feed and returns every event instance starting within [from, to)
// as wall-clock time in loc: single events as they are, repeating events
// expanded one instance each, with an instance the feed overrides replaced by
// its override.
func Parse(r io.Reader, loc *time.Location, from, to time.Time) ([]Event, error) {
	lines, err := unfold(r)
	if err != nil {
		return nil, err
	}
	components, err := parseComponents(lines)
	if err != nil {
		return nil, err
	}
	exceptions := map[string]map[int64]Event{}
	masters := []component{}
	for _, c := range components {
		rid, ok := c.first("RECURRENCE-ID")
		if !ok {
			masters = append(masters, c)
			continue
		}
		e, err := build(c, loc)
		if err != nil {
			return nil, err
		}
		orig, _, err := parseTime(rid, loc)
		if err != nil {
			return nil, fmt.Errorf("event %s recurrence-id: %w", e.UID, err)
		}
		e.Key = instanceKey(e.UID, orig, e.AllDay)
		e.Recurring = true
		if exceptions[e.UID] == nil {
			exceptions[e.UID] = map[int64]Event{}
		}
		exceptions[e.UID][orig.Unix()] = e
	}
	out := []Event{}
	within := func(t time.Time) bool {
		return !t.Before(from) && t.Before(to)
	}
	for _, c := range masters {
		e, err := build(c, loc)
		if err != nil {
			return nil, err
		}
		ruleProp, ok := c.first("RRULE")
		if !ok {
			if within(e.Start) {
				out = append(out, e)
			}
			continue
		}
		rr, err := parseRule(ruleProp.value, loc)
		if err != nil {
			return nil, fmt.Errorf("event %s rrule: %w", e.UID, err)
		}
		excluded := map[int64]bool{}
		for _, p := range c.all("EXDATE") {
			for _, v := range strings.Split(p.value, ",") {
				t, _, err := parseTime(property{value: v, params: p.params}, loc)
				if err != nil {
					return nil, fmt.Errorf("event %s exdate: %w", e.UID, err)
				}
				excluded[t.Unix()] = true
			}
		}
		duration := e.End.Sub(e.Start)
		for _, t := range rr.instances(e.Start, to) {
			if excluded[t.Unix()] {
				continue
			}
			if override, ok := exceptions[e.UID][t.Unix()]; ok {
				delete(exceptions[e.UID], t.Unix())
				if within(override.Start) {
					out = append(out, override)
				}
				continue
			}
			if !within(t) {
				continue
			}
			instance := e
			instance.Key = instanceKey(e.UID, t, e.AllDay)
			instance.Start = t
			instance.End = t.Add(duration)
			if e.AllDay {
				instance.End = t.AddDate(0, 0, int(duration.Hours()/24))
			}
			instance.Recurring = true
			out = append(out, instance)
		}
	}
	for _, byTime := range exceptions {
		for _, e := range byTime {
			if within(e.Start) {
				out = append(out, e)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Start.Equal(out[j].Start) {
			return out[i].Start.Before(out[j].Start)
		}
		return out[i].Key < out[j].Key
	})
	return out, nil
}
