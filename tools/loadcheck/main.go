// Command loadcheck loads the directory model from a sheet and prints a summary.
package main

import (
	"flag"
	"fmt"
	"log"
	"sort"

	"heliosian/internal/data"
	"heliosian/internal/directory"
)

func main() {
	sheet := flag.String("sheet", "", "spreadsheet id")
	flag.Parse()
	if *sheet == "" {
		log.Fatal("[ERROR] -sheet <spreadsheet id> is required")
	}
	source, err := data.NewSheet(map[string]string{"directory": *sheet})
	if err != nil {
		log.Fatalf("[ERROR] sheet source: %v", err)
	}
	model, err := directory.LoadModel(source, nil)
	if err != nil {
		log.Fatalf("[ERROR] load model: %v", err)
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
	fmt.Printf("families: %d\n", len(model.Families))
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
		fmt.Printf("  %s (image %v, crews %v)\n", c.Name, c.ImageURL != "", c.HasSections)
	}
	fmt.Println("crews:")
	for _, s := range model.Sections {
		fmt.Printf("  %s | %s | %s | teachers %v\n", s.Classroom, s.Name, s.GradeBand, s.Teachers)
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
	for _, email := range []string{"lexi.augenbergs@heliosschool.org", "daren.liang@heliosschool.org", "yeada.li@heliosschool.org", "dog@heliosschool.org", "mike.orlando@heliosschool.org", "evie.weiss@heliosschool.org"} {
		for _, p := range model.People {
			if p.Email == email {
				fmt.Printf("spot %s: full=%q legal=%q pref=%q roles=S%v/P%v/T%v grade=%q class=%q crew=%q band=%q dept=%q title=%q family=%s\n",
					email, p.FullName, p.LegalName, p.PreferredName, p.IsStudent, p.IsParent, p.IsStaff,
					p.Grade, p.Classroom, p.Section, p.GradeBand, p.Department, p.JobTitle, p.FamilyKey)
			}
		}
	}
}
