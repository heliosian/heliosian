package calendarimport

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"

	"github.com/anthropics/anthropic-sdk-go"

	"heliosian/internal/claude"
	"heliosian/internal/model"
)

const (
	modelName = "claude-fable-5-1"
	noDayType = "None"
	noMarker  = "None"
)

func ask(ctx context.Context, client anthropic.Client, system string, content []anthropic.ContentBlockParamUnion, schema map[string]any, effort anthropic.OutputConfigEffort, out any) (string, error) {
	return claude.JSON(ctx, client, anthropic.MessageNewParams{
		Model:     modelName,
		MaxTokens: 64000,
		System: []anthropic.TextBlockParam{{
			Text:         system,
			CacheControl: anthropic.NewCacheControlEphemeralParam(),
		}},
		Messages:     []anthropic.MessageParam{anthropic.NewUserMessage(content...)},
		OutputConfig: anthropic.OutputConfigParam{Effort: effort, Format: anthropic.JSONOutputFormatParam{Schema: schema}},
	}, out)
}

func retryOnce(ctx context.Context, again string, ask func() (string, error), check func(raw string) error) error {
	for attempt := 1; ; attempt++ {
		raw, err := ask()
		if err != nil {
			return err
		}
		err = check(raw)
		if err == nil {
			return nil
		}
		if attempt == 2 {
			return err
		}
		slog.WarnContext(ctx, again, "error", err)
	}
}

func fanOut[T any](n int, f func(i int) (T, error)) ([]T, []error) {
	results := make([]T, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() { results[i], errs[i] = f(i) })
	}
	wg.Wait()
	return results, errs
}

func enumOf(values []string) map[string]any {
	return map[string]any{"type": "string", "enum": values}
}

func glossary(roster model.Roster) string {
	b := &strings.Builder{}
	b.WriteString("Helios School is a K-8 school. Students belong to a homeroom classroom named for a bird. Two classrooms make a grade band whose name is a portmanteau of the two classroom names. Lower School is Kindergarten through Grade 4 and Middle School is Grade 5 through Grade 8.\n\nBands and their classrooms:\n")
	bands := []string{}
	for _, c := range roster.Classrooms {
		if c.Band != "" && !slices.Contains(bands, c.Band) {
			bands = append(bands, c.Band)
		}
	}
	for _, band := range bands {
		parts := []string{}
		for _, c := range roster.Classrooms {
			if c.Band == band {
				parts = append(parts, c.Name+" ("+strings.Join(c.Grades, ", ")+")")
			}
		}
		fmt.Fprintf(b, "- %s = %s\n", band, strings.Join(parts, " + "))
	}
	b.WriteString("\nClassrooms with crews (a crew is a smaller group within a classroom, written as crew then classroom):\n")
	for _, c := range roster.Classrooms {
		if len(c.Crews) > 0 {
			fmt.Fprintf(b, "- %s: %s\n", c.Name, strings.Join(c.Crews, ", "))
		}
	}
	b.WriteString(`
Vocabulary seen in event titles:
- K or Kinder means Kindergarten. "1/2", "3/4", "5/6", "7/8" name grade pairs, which are bands. "8th Grade" means Grade 8. LS is Lower School, MS is Middle School.
- A singular band name (Cosprey, Hegret, Jayven, Halcon) means the band.
- CoL or COL is a Celebration of Learning, a showcase where students present their work to families.
- ILP is an Individual Learning Plan conference between a family and teachers.
- MAP is the MAP Growth assessment students take.
- BTSN or Back to School Night is an evening for parents.
- CAFE is a morning coffee for the parents of one band or classroom.
- Beacon Wellness is the school's counseling program; its chats are for parents.
- HCA is the Helios Community Association, the parent association; its events are for parents and families.
- PD or Professional Development is a day or half day staff work while students stay home or leave early.
- Intersession is a week of alternative programming, still a school day.
- Passion Projects are Middle School project weeks.
- Aftercare is the after-school care program.
`)
	return b.String()
}

const legendSystem = `You are reading an image of a school's one-page year calendar. Somewhere on the page, usually near the bottom, is a legend: small color swatches, each beside a line of text saying what a day cell filled with that color means. Read the legend only.

For each swatch: the exact wording beside it; the swatch's fill color as a hex RGB estimate like #c9b7e6; and the day type the wording means for students, from the allowed list: a day students stay home is "No School", a day they are released early is "Early Dismissal", and a wording that means neither is "None". Headings such as "NO SCHOOL DAYS" or "HALF SCHOOL DAYS" above the swatches are not entries.`

func monthSystem(described string) string {
	return `You are reading one month grid of a school's year calendar. Some day cells have a colored background, and the calendar's legend gives each color a meaning:
` + described + `
Report the month named in the grid's title. Then go through the grid row by row; each row is one week. For every day number printed, report its cell's background: "` + unfilled + `" for a white cell; the legend wording whose color the fill matches when it is one of the legend's colors; "` + otherFill + `" for a fill in a color the legend does not name, such as orange or yellow. Some cells have a thick colored border drawn around them; a border is a box on top of the cell, not its fill, so report the color inside the border: a cell that is outlined, bolded, or circled but white inside is "` + unfilled + `", and a cell with a legend color inside an orange or black border is that legend color.`
}

func entriesSystem(roster model.Roster) string {
	return glossary(roster) + `
You are reading the school's one-page year calendar PDF: twelve month grids, a legend, and an Important Dates list. Read the Important Dates list into entries.

- One entry for every line of the list, with its exact wording as the title and the date range it gives. A range like "Aug 24-Sep 2" runs across the month boundary. "Mar 11+18" is two entries, one per day.
- dayType comes only from the wording: a dismissal note such as "12:30 dismissal", "12:30p Dismissals", or "half day" means "Early Dismissal"; wording that says school is closed means "No School"; anything else is "None". Ignore the color a line is printed in. An evening event's hours are not a dismissal note.
- classrooms is every classroom the entry applies to: all of them unless the wording limits it, such as "(K only)", which is the kindergarten classroom alone.
- marker is "First Day" on the first day of school for students, "Last Day" on the last day of school, and "None" otherwise.
- year is the school year in the title, written as YYYY-YYYY. Dates are YYYY-MM-DD; the year of each date follows from which side of the winter break the month is on.`
}

func classifierSystem(roster model.Roster, tags []model.CalendarTag) string {
	described := &strings.Builder{}
	for _, t := range tags {
		fmt.Fprintf(described, "- %s: %s\n", t.Name, t.Description)
	}
	return glossary(roster) + `
You classify events from the school calendar for a family-facing app. For each event, using only its title, dates, location and description:

- tags: every tag that applies. First the classrooms the event is for, as the narrowest set the event supports: a band's two classrooms when the title names a band, a grade pair, or both classrooms; one classroom when it names one; the Lower School or Middle School classrooms when it says LS or MS; every classroom when nothing narrows it. An event for parents or staff is still for the classrooms whose families or staff it concerns, every classroom when school-wide. Then every one of these that applies:
` + described.String() + `- dayType: for an all-day event only, the day type it imposes on the students it applies to, or "None". "No School" for holidays, breaks and professional development days; "Early Dismissal" for early dismissal and half days; any other listed type only when the title says so plainly. An event with a time of day, or one that merely happens on a school day, is "None".
- keywords: three to eight lowercase search words a parent might type that are in neither the title nor the tags: expansions of abbreviations, synonyms, the occasion, what happens there. Never a classroom, band, or grade, and never a tag.

Answer under every event's id, repeating its title exactly as given.`
}
