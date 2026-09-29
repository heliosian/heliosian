package main

import (
	"flag"
	"fmt"
	"log"
	"sort"
	"time"

	"heliosian/internal/artifacts"
	"heliosian/internal/blob"
	"heliosian/internal/celebrate"
	"heliosian/internal/data"
	"heliosian/internal/devcache"
	"heliosian/internal/env"
	"heliosian/internal/home"
	"heliosian/internal/loop"
	"heliosian/internal/model"
	"heliosian/internal/spreadsheets"
	"heliosian/internal/static"
	"heliosian/internal/store"
	"heliosian/internal/team"
	"heliosian/internal/when"
)

func main() {
	dir := flag.String("dir", "", "load from a directory of dumped tabs instead of the live sheets")
	flag.Parse()
	devcache.Install()
	var source data.Source
	if *dir != "" {
		source = &data.Dir{Root: *dir}
	} else {
		live, err := data.NewSheet(spreadsheets.IDs(spreadsheets.Of([]string{"directory", "preferences", "invites", "apps", "events", "celebrate", "calendar", "config", "groups", "artifacts"})))
		if err != nil {
			log.Fatalf("sheet source: %v", err)
		}
		source = live
	}
	objects, err := blob.Open(blob.MediaBucket)
	if err != nil {
		log.Fatalf("blob store: %v", err)
	}
	media := blob.New(objects)
	directory, err := model.LoadDirectory(source, media, static.Files{Root: "web/who"}, []byte(env.Required("ID_KEY")))
	if err != nil {
		log.Fatalf("load directory model: %v", err)
	}
	students, parents, staff, isNew := 0, 0, 0, 0
	for _, p := range directory.People {
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
		len(directory.People), students, parents, staff, isNew)
	byStatus := map[model.OptStatus]int{}
	addressMasked, phoneMasked := 0, 0
	for _, p := range directory.People {
		byStatus[p.OptStatus]++
		if p.AddressMasked {
			addressMasked++
		}
		if p.PhoneMasked {
			phoneMasked++
		}
	}
	fmt.Printf("preferences: opt in %d, opt out %d, default %d (address masked %d, phone masked %d)\n",
		byStatus[model.OptIn], byStatus[model.OptOut], byStatus[model.OptDefault],
		addressMasked, phoneMasked)
	familyAddressMasked, familyPhoneMasked := 0, 0
	for _, f := range directory.Families {
		if f.AddressMasked {
			familyAddressMasked++
		}
		if f.PhoneMasked {
			familyPhoneMasked++
		}
	}
	fmt.Printf("families: %d (address masked %d, phone masked %d)\n",
		len(directory.Families), familyAddressMasked, familyPhoneMasked)
	twoHousehold := map[string]int{}
	for _, f := range directory.Families {
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
	for _, c := range directory.Classrooms {
		fmt.Printf("  %s (image %v, crews %v)\n", c.Name, c.ImageURL != "", c.HasCrews)
	}
	fmt.Println("crews:")
	for _, c := range directory.Crews {
		fmt.Printf("  %s | %s | %s | teachers %v\n", c.Classroom, c.Name, c.GradeBand, c.Teachers)
	}
	bands := []string{}
	for band := range directory.RoomParents {
		bands = append(bands, band)
	}
	sort.Strings(bands)
	fmt.Println("room parents:")
	for _, band := range bands {
		fmt.Printf("  %s: %d\n", band, len(directory.RoomParents[band]))
	}
	fmt.Println("departments:", directory.Departments)
	invites, err := model.NewInviteTemplatesCache(source, nil, store.NewQueue())
	if err != nil {
		log.Fatalf("load invites: %v", err)
	}
	fmt.Printf("invites: %d systems, %d greetings\n", len(invites.Model().Systems), len(invites.Model().Greetings))
	fmt.Println("grades:")
	for _, g := range directory.Grades {
		fmt.Printf("  %s -> %s (%s -> %s)\n", g.Name, g.NextName, g.Band, g.NextBand)
	}

	appsCache, err := home.NewCache(source, nil, blob.NewImages(media, "home"), func() []string { return nil }, nil, store.NewQueue())
	if err != nil {
		log.Fatalf("load apps model: %v", err)
	}
	apps := appsCache.Model()
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
	fmt.Printf("apps admins: %d\n", len(appsCache.Admins()))

	eventsCache, err := team.NewCache(source, nil, blob.NewImages(media, "team"), func() []string { return nil }, store.NewQueue())
	if err != nil {
		log.Fatalf("load events model: %v", err)
	}
	portal := eventsCache.Model()
	fmt.Println("events:")
	byYear := map[string][]*team.Activity{}
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
	fmt.Printf("events admins: %d\n", len(eventsCache.Admins()))

	celebrateCache, err := celebrate.NewCache(source, nil, blob.NewImages(media, "celebrate"), func() []string { return nil }, store.NewQueue())
	if err != nil {
		log.Fatalf("load celebrate model: %v", err)
	}
	site := celebrateCache.Model()
	fmt.Println("celebrate:")
	for _, c := range site.Celebrations {
		sold, waiting, hosts := 0, 0, 0
		parties := site.SortedParties(c.ID)
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
	fmt.Printf("celebrate admins: %d\n", len(celebrateCache.Admins()))

	configCache, err := model.NewConfigCache(source, nil, store.NewQueue())
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	settings := configCache.Config()
	fmt.Printf("config: %d super admins, stale years %+v, staff color %s, %d grade colors, %d classroom colors\n",
		len(settings.SuperAdmins), settings.StaleYears, settings.StaffColor, len(settings.GradeColors), len(settings.ClassroomColors))

	calendarCache, err := when.NewCache(source, nil, func() when.Roster { return when.RosterOf(directory) }, nil, func() []string { return nil }, store.NewQueue())
	if err != nil {
		log.Fatalf("load calendar model: %v", err)
	}
	plan := calendarCache.Model()
	bySource, byTag := map[string]int{}, map[string]int{}
	for _, e := range plan.Events {
		bySource[e.Source]++
		for _, t := range e.Tags {
			byTag[t]++
		}
	}
	fmt.Printf("calendar: %d events (google %d, pdf %d, sheet %d), %d hidden, %d duplicates folded\n",
		len(plan.Events), bySource[when.SourceGoogle], bySource[when.SourcePDF], bySource[when.SourceSheet], plan.Hidden, plan.Duplicates)
	for _, t := range plan.Tags {
		fmt.Printf("  %s: %d\n", t.Name, byTag[t.ID])
	}
	for reason, n := range plan.Skipped {
		fmt.Printf("  skipped %d: %s\n", n, reason)
	}
	for _, y := range plan.Years {
		fmt.Printf("  school year %s: %s to %s\n", y.Label, y.FirstDay, y.LastDay)
	}
	byType := map[string]int{}
	for _, byClassroom := range plan.Days {
		for _, key := range byClassroom {
			byType[key]++
		}
	}
	fmt.Printf("  day plan: %d dates\n", len(plan.Days))
	for _, d := range plan.DayTypes {
		fmt.Printf("    %s: %d classroom-days\n", d.Name, byType[d.ID])
	}
	fmt.Printf("calendar admins: %d\n", len(calendarCache.Admins()))

	groupCache, err := loop.NewCache(source, nil, func() []string { return nil }, store.NewQueue(), []byte(env.Required("ID_KEY")))
	if err != nil {
		log.Fatalf("load groups model: %v", err)
	}
	groupModel := groupCache.Model()
	now := time.Now().In(when.Location)
	sources := loop.Sources{
		Directory: directory,
		Tags:      directory.Tags,
		MagicTags: func(owner string) []model.MagicTag {
			lists := append(directory.RoomParentTags(owner), site.Lists(directory, owner, now)...)
			return append(lists, portal.Lists(directory, owner, now)...)
		},
		Shared: directory.SharedTags,
	}
	fmt.Println("groups:")
	for _, g := range groupModel.Groups {
		fmt.Printf("  %s %q: aliases %v, %d managers, %d rules, %d members, %d excluded, prefix %v, visibility %s, posting %s, replying %s\n", g.Address(), g.Title, g.Aliases, len(g.Managers), len(g.Rules), len(loop.Members(g, sources)), len(g.Excluded), g.Prefix, g.Visibility, g.Posting, g.Replying)
	}
	fmt.Printf("groups admins: %d\n", len(groupCache.Admins()))

	embedder, err := artifacts.NewVertex()
	if err != nil {
		log.Fatalf("embedder: %v", err)
	}
	artifactsCache, err := artifacts.NewCache(source, nil, objects, embedder, store.NewQueue())
	if err != nil {
		log.Fatalf("load artifacts: %v", err)
	}
	documents := artifactsCache.Model().Documents
	fmt.Printf("artifacts: %d documents\n", len(documents))
	for _, doc := range documents {
		fmt.Printf("  %s %q by %s (%s, %d chunks)\n", doc.Date, doc.Title, doc.Author, doc.Kind, len(doc.Chunks))
	}
}
