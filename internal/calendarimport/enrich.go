package calendarimport

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"

	"heliosian/internal/cells"
	"heliosian/internal/when"
)

const batchSize = 10

type enrichInput struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Start       string `json:"start"`
	End         string `json:"end"`
	Location    string `json:"location,omitempty"`
	Description string `json:"description,omitempty"`
}

type enrichOutput struct {
	ID       string   `json:"-"`
	Title    string   `json:"title"`
	Tags     []string `json:"tags"`
	DayType  string   `json:"dayType"`
	Keywords []string `json:"keywords"`
}

func digest(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return fmt.Sprintf("%x", sum)[:16]
}

func enrich(ctx context.Context, client anthropic.Client, inputs []enrichInput, roster when.Roster, dayTypes []string, tags []when.Tag) ([]enrichOutput, error) {
	names := []string{}
	for _, t := range tags {
		names = append(names, t.Name)
	}
	system := classifierSystem(roster, tags)
	sameAs := map[string]string{}
	representatives := []enrichInput{}
	for _, in := range inputs {
		text := digest(in.Title, in.Location, in.Description)
		if first, ok := sameAs[text]; ok {
			sameAs[in.ID] = first
			continue
		}
		sameAs[text] = in.ID
		sameAs[in.ID] = in.ID
		representatives = append(representatives, in)
	}
	answer := map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"title", "tags", "dayType", "keywords"},
		"properties": map[string]any{
			"title":    map[string]any{"type": "string", "description": "the event's title, exactly as given"},
			"tags":     map[string]any{"type": "array", "minItems": 1, "items": enumOf(append(roster.Names(), names...))},
			"dayType":  enumOf(append([]string{noDayType}, dayTypes...)),
			"keywords": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
	}
	sent := []enrichInput{}
	byHandle := map[string]string{}
	properties := map[string]any{}
	required := []string{}
	for i, in := range representatives {
		handle := "e" + strconv.Itoa(i+1)
		byHandle[handle] = in.ID
		in.ID = handle
		sent = append(sent, in)
		properties[handle] = map[string]any{"$ref": "#/$defs/answer"}
		required = append(required, handle)
	}
	schema := map[string]any{
		"type": "object", "additionalProperties": false, "required": required, "properties": properties,
		"$defs": map[string]any{"answer": answer},
	}
	encoded, err := json.MarshalIndent(sent, "", " ")
	if err != nil {
		return nil, err
	}
	content := []anthropic.ContentBlockParamUnion{
		anthropic.NewTextBlock("Classify each of these events:\n\n" + string(encoded)),
	}
	answers := map[string]enrichOutput{}
	out := map[string]enrichOutput{}
	err = retryOnce(ctx, "calendar import: asking for the classification again",
		func() (string, error) {
			out = map[string]enrichOutput{}
			return ask(ctx, client, system, content, schema, anthropic.OutputConfigEffort("max"), &out)
		},
		func(raw string) error {
			for i, in := range representatives {
				o := out["e"+strconv.Itoa(i+1)]
				if collapse(o.Title) != collapse(in.Title) {
					return fmt.Errorf("claude answered %s with the title %q, not %q; the answer was %s", in.ID, o.Title, in.Title, raw)
				}
				o.ID = in.ID
				answers[in.ID] = o
			}
			return nil
		})
	if err != nil {
		return nil, err
	}
	ordered := []enrichOutput{}
	for _, in := range inputs {
		o := answers[sameAs[in.ID]]
		o.ID = in.ID
		ordered = append(ordered, o)
	}
	return ordered, nil
}

func plan(existing map[string]map[string]string, rows []map[string]string, vocabulary string) ([]map[string]string, []enrichInput, map[string]string) {
	kept := []map[string]string{}
	pending := []enrichInput{}
	hashes := map[string]string{}
	for _, row := range rows {
		id := row["Event ID"]
		hash := digest(row["Title"], row["Description"], row["Start"], row["End"], row["Location"], vocabulary)
		if have, ok := existing[id]; ok && have["Input Hash"] == hash {
			kept = append(kept, have)
			continue
		}
		pending = append(pending, enrichInput{ID: id, Title: row["Title"], Start: row["Start"], End: row["End"], Location: row["Location"], Description: row["Description"]})
		hashes[id] = hash
	}
	return kept, pending, hashes
}

func (r *run) enrich(ctx context.Context, rows []map[string]string, pdf bool) []map[string]string {
	vocabulary := digest(classifierSystem(r.roster, r.tags), strings.Join(r.roster.Names(), ","), strings.Join(r.dayTypes, ","))
	existing := map[string]map[string]string{}
	for _, row := range r.tables[when.EnrichmentTab] {
		existing[row["Event ID"]] = row
	}
	enrichment, pending, hashes := plan(existing, rows, vocabulary)
	slog.InfoContext(ctx, "calendar import: enrichment", "current", len(enrichment), "to classify", len(pending))
	today := time.Now().In(when.Location).Format(when.DateFormat)
	batches := [][]enrichInput{}
	for start := 0; start < len(pending); start += batchSize {
		batches = append(batches, pending[start:min(start+batchSize, len(pending))])
	}
	results, errs := fanOut(len(batches), func(i int) ([]enrichOutput, error) {
		return enrich(ctx, r.client, batches[i], r.roster, r.dayTypes, r.tags)
	})
	for i, batch := range batches {
		if errs[i] != nil {
			slog.ErrorContext(ctx, "calendar import: classify events", "batch", i+1, "of", len(batches), "error", errs[i])
			r.failures = append(r.failures, fmt.Sprintf("classification batch %d of %d", i+1, len(batches)))
			continue
		}
		for _, a := range results[i] {
			if pdf {
				a.DayType = noDayType
			}
			row, err := r.enrichmentRow(a, hashes[a.ID], today)
			if err != nil {
				slog.ErrorContext(ctx, "calendar import: classification names what the sheet does not have", "event", a.ID, "error", err)
				r.failures = append(r.failures, "classifying "+a.ID)
				continue
			}
			enrichment = append(enrichment, row)
		}
		slog.InfoContext(ctx, "calendar import: batch classified", "batch", i+1, "of", len(batches), "events", len(batch))
	}
	byTag := map[string]int{}
	for _, row := range enrichment {
		for _, t := range cells.SplitList(row["Tags"]) {
			byTag[t]++
		}
	}
	for _, t := range r.tags {
		if byTag[t.ID] > 0 {
			slog.InfoContext(ctx, "calendar import: tag count", "tag", t.Name, "events", byTag[t.ID])
		}
	}
	return enrichment
}

func (r *run) enrichmentRow(a enrichOutput, hash, today string) (map[string]string, error) {
	tags, err := r.tagCell(a.Tags)
	if err != nil {
		return nil, err
	}
	row := map[string]string{
		"Event ID": a.ID, "Tags": tags,
		"Keywords": cells.JoinList(a.Keywords), "Input Hash": hash, "Model": modelName, "Enriched": today,
	}
	if a.DayType == noDayType {
		return row, nil
	}
	if row["Day Type"], err = r.dayTypeID(a.DayType); err != nil {
		return nil, err
	}
	return row, nil
}
