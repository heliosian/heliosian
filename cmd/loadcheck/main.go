// Command loadcheck runs every app's load pipeline against the production sheets and prints a summary.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"heliosian/internal/celebrate"
	"heliosian/internal/app"
	"heliosian/internal/calendar"
	"heliosian/internal/config"
	"heliosian/internal/data"
	"heliosian/internal/events"
	"heliosian/internal/home"
	"heliosian/internal/who"
)

type staticFiles struct {
	root string
}

func (s staticFiles) Has(key string) (bool, error) {
	_, err := os.Stat(filepath.Join(s.root, filepath.FromSlash(key)))
	return err == nil, nil
}

func (staticFiles) Prefetch([]string) error { return nil }

func bundled(roots []string, key string) bool {
	for _, root := range roots {
		if found, _ := (staticFiles{root}).Has(key); found {
			return true
		}
	}
	return false
}

// homeImages trusts bucket names, since this tool carries no bucket client,
// and checks bundled files on disk.
type homeImages struct{}

func (homeImages) Has(key string) (bool, error) {
	if strings.HasPrefix(key, "link-images/") {
		return true, nil
	}
	return bundled([]string{"web/home", "web/public/home"}, key), nil
}

func (homeImages) Prefetch([]string) error { return nil }

// eventsImages trusts bucket names like homeImages does, and checks bundled files.
type eventsImages struct{}

func (eventsImages) Has(key string) (bool, error) {
	if strings.HasPrefix(key, "activity-images/") {
		return true, nil
	}
	return bundled([]string{"web/team", "web/public/team"}, key), nil
}

func (eventsImages) Prefetch([]string) error { return nil }

// celebrateImages trusts bucket names like the others, and checks bundled files.
type celebrateImages struct{}

func (celebrateImages) Has(key string) (bool, error) {
	if strings.HasPrefix(key, "party-images/") {
		return true, nil
	}
	return bundled([]string{"web/celebrate", "web/public/celebrate"}, key), nil
}

func (celebrateImages) Prefetch([]string) error { return nil }

func requiredEnv(name string) string {
	value := os.Getenv(name)
	if value == "" {
		log.Fatalf("[ERROR] %s is required", name)
	}
	return value
}

func main() {
	dir := flag.String("dir", "", "load from a directory of dumped tabs instead of the live sheets")
	flag.Parse()
	var source data.Source
	if *dir != "" {
		source = &data.Dir{Root: *dir}
	} else {
		live, err := data.NewSheet(map[string]string{
			"directory":   requiredEnv("DIRECTORY_SHEET"),
			"preferences": requiredEnv("PREFERENCES_SHEET"),
			"invites":     requiredEnv("INVITES_SHEET"),
			"apps":        requiredEnv("APPS_SHEET"),
			"events":      requiredEnv("EVENTS_SHEET"),
			"celebrate":   requiredEnv("CELEBRATE_SHEET"),
			"calendar":    requiredEnv("CALENDAR_SHEET"),
			"config":      requiredEnv("CONFIG_SHEET"),
		})
		if err != nil {
			log.Fatalf("[ERROR] sheet source: %v", err)
		}
		source = live
	}
	model, err := who.LoadModel(source, nil, staticFiles{"web/who"})
	if err != nil {
		log.Fatalf("[ERROR] load directory model: %v", err)
	}
	students, parents, staff, isNew := 0, 0, 0, 0
	for _, p := range model.People {
		if p.IsStudent {
			students++
		}
		if p.IsParent {
			parents++
		}
		if p.IsStaff {
			staff++
		}
		if p.IsNew {
			isNew++
		}
	}
	fmt.Printf("people: %d (students %d, parents %d, staff %d, new %d)\n",
		len(model.People), students, parents, staff, isNew)
	byStatus := map[who.OptStatus]int{}
	addressMasked, phoneMasked := 0, 0
	for _, p := range model.People {
		byStatus[p.OptStatus]++
		if p.AddressMasked {
			addressMasked++
		}
		if p.PhoneMasked {
			phoneMasked++
		}
	}
	fmt.Printf("preferences: opt in %d, opt out %d, default %d (address masked %d, phone masked %d)\n",
		byStatus[who.OptIn], byStatus[who.OptOut], byStatus[who.OptDefault],
		addressMasked, phoneMasked)
	familyAddressMasked, familyPhoneMasked := 0, 0
	for _, f := range model.Families {
		if f.AddressMasked {
			familyAddressMasked++
		}
		if f.PhoneMasked {
			familyPhoneMasked++
		}
	}
	fmt.Printf("families: %d (address masked %d, phone masked %d)\n",
		len(model.Families), familyAddressMasked, familyPhoneMasked)
	twoHousehold := map[string]int{}
	for _, f := range model.Families {
		for _, kid := range f.KidEmails {
			twoHousehold[kid]++
		}
	}
	for kid, n := range twoHousehold {
		if n > 1 {
			fmt.Printf("  student in %d households: %s\n", n, kid)
		}
	}
	fmt.Println("classrooms:")
	for _, c := range model.Classrooms {
		fmt.Printf("  %s (image %v, crews %v)\n", c.Name, c.ImageURL != "", c.HasCrews)
	}
	fmt.Println("crews:")
	for _, c := range model.Crews {
		fmt.Printf("  %s | %s | %s | teachers %v\n", c.Classroom, c.Name, c.GradeBand, c.Teachers)
	}
	bands := []string{}
	for band := range model.RoomParents {
		bands = append(bands, band)
	}
	sort.Strings(bands)
	fmt.Println("room parents:")
	for _, band := range bands {
		fmt.Printf("  %s: %d\n", band, len(model.RoomParents[band]))
	}
	fmt.Println("departments:", model.Departments)
	fmt.Println("grades:")
	for _, g := range model.Grades {
		fmt.Printf("  %s -> %s (%s -> %s)\n", g.Name, g.NextName, g.Band, g.NextBand)
	}

	tables, err := home.ReadTables(source)
	if err != nil {
		log.Fatalf("[ERROR] read apps tables: %v", err)
	}
	apps, err := home.BuildModel(tables, homeImages{})
	if err != nil {
		log.Fatalf("[ERROR] build apps model: %v", err)
	}
	fmt.Println("apps:")
	for _, c := range apps.Categories {
		fmt.Printf("  %s %s: %d links\n", c.Title, c.Emoji, len(c.Links))
		for _, l := range c.Links {
			visible := "visible"
			if !l.Visible {
				visible = "hidden"
			}
			fmt.Printf("    %s -> %s (%s, image %v)\n", l.Title, l.URL, visible, l.ImageURL != "")
		}
	}
	fmt.Printf("apps admins: %d\n", len(tables.Admins))

	eventTables, err := events.ReadTables(source)
	if err != nil {
		log.Fatalf("[ERROR] read events tables: %v", err)
	}
	portal, err := events.BuildModel(eventTables, eventsImages{})
	if err != nil {
		log.Fatalf("[ERROR] build events model: %v", err)
	}
	fmt.Println("events:")
	byYear := map[string][]*events.Activity{}
	years := []string{}
	for _, a := range portal.Activities {
		if _, seen := byYear[a.Year]; !seen {
			years = append(years, a.Year)
		}
		byYear[a.Year] = append(byYear[a.Year], a)
	}
	sort.Strings(years)
	for _, year := range years {
		fmt.Printf("  %s: %d activities\n", year, len(byYear[year]))
		for _, a := range byYear[year] {
			volunteers := len(a.Volunteers)
			for _, c := range a.Descendants() {
				volunteers += len(c.Volunteers)
			}
			fmt.Printf("    %s [%s, %s] under it %d, links %d, volunteers %d, image %v\n",
				a.Title, a.Category, a.Status, len(a.Descendants()), len(a.Links), volunteers, a.ImageURL != "")
		}
	}
	fmt.Printf("events admins: %d\n", len(eventTables.Admins))

	celebrateTables, err := celebrate.ReadTables(source)
	if err != nil {
		log.Fatalf("[ERROR] read celebrate tables: %v", err)
	}
	site, err := celebrate.BuildModel(celebrateTables, celebrateImages{})
	if err != nil {
		log.Fatalf("[ERROR] build celebrate model: %v", err)
	}
	fmt.Println("celebrate:")
	for _, c := range site.Celebrations {
		sold, waiting, hosts := 0, 0, 0
		parties := site.SortedParties(c.Code)
		for _, p := range parties {
			hosts += len(p.HostEmails)
			for _, t := range p.Tickets {
				if t.Status == celebrate.TicketWaitlist {
					waiting++
				} else {
					sold++
				}
			}
		}
		fmt.Printf("  %s %q (current %v): %d parties, %d hosts, %d tickets sold, %d waiting\n", c.Code, c.Title, c.Current, len(parties), hosts, sold, waiting)
	}
	for reason, n := range site.Skipped {
		fmt.Printf("  skipped %d: %s\n", n, reason)
	}
	fmt.Printf("celebrate admins: %d\n", len(celebrateTables.Admins))

	configTables, err := config.ReadTables(source)
	if err != nil {
		log.Fatalf("[ERROR] read config tables: %v", err)
	}
	settings, err := config.Parse(configTables)
	if err != nil {
		log.Fatalf("[ERROR] parse config: %v", err)
	}
	fmt.Printf("config: %d super admins, stale years %+v, staff color %s, %d grade colors, %d classroom colors\n",
		len(settings.SuperAdmins), settings.StaleYears, settings.StaffColor, len(settings.GradeColors), len(settings.ClassroomColors))

	calendarTables, err := calendar.ReadTables(source)
	if err != nil {
		log.Fatalf("[ERROR] read calendar tables: %v", err)
	}
	plan, err := calendar.BuildModel(calendarTables, app.CalendarRoster(model))
	if err != nil {
		log.Fatalf("[ERROR] build calendar model: %v", err)
	}
	bySource, byTag := map[string]int{}, map[string]int{}
	for _, e := range plan.Events {
		bySource[e.Source]++
		for _, t := range e.Tags {
			byTag[t]++
		}
	}
	fmt.Printf("calendar: %d events (google %d, pdf %d, sheet %d), %d hidden\n",
		len(plan.Events), bySource[calendar.SourceGoogle], bySource[calendar.SourcePDF], bySource[calendar.SourceSheet], plan.Hidden)
	for _, t := range plan.Tags {
		fmt.Printf("  %s: %d\n", t.Name, byTag[t.Name])
	}
	for reason, n := range plan.Skipped {
		fmt.Printf("  skipped %d: %s\n", n, reason)
	}
	for _, y := range plan.Years {
		fmt.Printf("  school year %s: %s to %s\n", y.Label, y.FirstDay, y.LastDay)
	}
	byType := map[string]int{}
	for _, byClassroom := range plan.Days {
		for _, name := range byClassroom {
			byType[name]++
		}
	}
	fmt.Printf("  day plan: %d dates\n", len(plan.Days))
	for _, d := range plan.DayTypes {
		fmt.Printf("    %s: %d classroom-days\n", d.Name, byType[d.Name])
	}
	fmt.Printf("calendar admins: %d\n", len(calendarTables.Admins))
}
