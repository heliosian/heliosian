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

	"heliosian/internal/config"
	"heliosian/internal/data"
	"heliosian/internal/home"
	"heliosian/internal/who"
)

type staticFiles struct {
	root string
}

func (s staticFiles) Has(key string) bool {
	_, err := os.Stat(filepath.Join(s.root, filepath.FromSlash(key)))
	return err == nil
}

// homeImages trusts bucket names, since this tool carries no bucket client,
// and checks bundled files on disk.
type homeImages struct{}

func (homeImages) Has(key string) bool {
	if strings.HasPrefix(key, "link-images/") {
		return true
	}
	for _, root := range []string{"web/home", "web/public/home"} {
		if (staticFiles{root}).Has(key) {
			return true
		}
	}
	return false
}

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
		fmt.Printf("  %s (image %v): %d links\n", c.Title, c.ImageURL != "", len(c.Links))
		for _, l := range c.Links {
			visible := "visible"
			if !l.Visible {
				visible = "hidden"
			}
			fmt.Printf("    %s -> %s (%s, image %v)\n", l.Title, l.URL, visible, l.ImageURL != "")
		}
	}
	fmt.Printf("apps admins: %d\n", len(tables.Admins))

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
}
