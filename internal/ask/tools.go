package ask

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"

	"heliosian/internal/model"
)

const maxToolOutput = 40000

type tool struct {
	name        string
	description string
	words       string
	properties  map[string]any
	required    []string
	run         func(t *turn, input json.RawMessage) (any, error)
}

func str(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func boolean(description string) map[string]any {
	return map[string]any{"type": "boolean", "description": description}
}

func integer(description string) map[string]any {
	return map[string]any{"type": "integer", "description": description}
}

func definitions() []anthropic.BetaToolUnionParam {
	out := []anthropic.BetaToolUnionParam{}
	for _, t := range tools {
		properties := t.properties
		if properties == nil {
			properties = map[string]any{}
		}
		out = append(out, anthropic.BetaToolUnionParam{OfTool: &anthropic.BetaToolParam{
			Name:        t.name,
			Description: anthropic.String(t.description),
			InputSchema: anthropic.BetaToolInputSchemaParam{Properties: properties, Required: t.required},
		}})
	}
	return out
}

func label(name string) string {
	for _, t := range tools {
		if t.name == name {
			return t.words
		}
	}
	return "Looking something up"
}

func (t *turn) run(ctx context.Context, name string, input json.RawMessage) (string, error) {
	i := slices.IndexFunc(tools, func(t tool) bool { return t.name == name })
	if i < 0 {
		return "", fmt.Errorf("there is no tool called %s", name)
	}
	if len(input) == 0 {
		input = json.RawMessage(`{}`)
	}
	scoped := *t
	scoped.r = t.r.WithContext(ctx)
	result, err := tools[i].run(&scoped, input)
	if err != nil {
		return "", err
	}
	encoded := &bytes.Buffer{}
	encoder := json.NewEncoder(encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(result); err != nil {
		return "", err
	}
	if encoded.Len() > maxToolOutput {
		return "", fmt.Errorf("that is too much at once (%d characters); narrow it with a name, a date range, a classroom or a smaller limit", encoded.Len())
	}
	return strings.TrimSpace(encoded.String()), nil
}

func decodeInput[T any](input json.RawMessage) (T, error) {
	var in T
	if err := json.Unmarshal(input, &in); err != nil {
		return in, fmt.Errorf("the input could not be read: %w", err)
	}
	return in, nil
}

func limitOf(n, fallback, ceiling int) int {
	if n <= 0 {
		return fallback
	}
	return min(n, ceiling)
}

func params(pairs ...string) url.Values {
	out := url.Values{}
	for i := 0; i+1 < len(pairs); i += 2 {
		if value := strings.TrimSpace(pairs[i+1]); value != "" {
			out.Set(pairs[i], value)
		}
	}
	return out
}

func collection(name string, v url.Values) string {
	return "/api/" + name + "?" + v.Encode()
}

func one(name, key string, v url.Values) string {
	return "/api/" + name + "/" + url.PathEscape(strings.TrimSpace(key)) + "?" + v.Encode()
}

func shown(limit int) string {
	return strconv.Itoa(limit + 1)
}

func first(list any) map[string]any {
	items, _ := list.([]any)
	if len(items) == 0 {
		return map[string]any{}
	}
	return items[0].(map[string]any)
}

func today(now time.Time) time.Time {
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, model.Location)
}

func date(cell string) (time.Time, error) {
	t, err := time.ParseInLocation(model.DateFormat, strings.TrimSpace(cell), model.Location)
	if err != nil {
		return t, fmt.Errorf("%q is not a date like 2026-09-24", cell)
	}
	return t, nil
}

var tools = []tool{findPeople, getPerson, getFamily, nearbyFamilies, getClassroom, calendarEvents, dayPlan, volunteerOpportunities, getActivity, parties, myGroups, myLists, communityLinks, searchDocuments, readDocument}
