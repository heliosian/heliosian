// Command loadcheck loads the directory model from a sheet and prints a summary.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"

	"heliosian/internal/data"
	"heliosian/internal/directory"
)

type staticFiles struct{}

func (staticFiles) Has(key string) bool {
	_, err := os.Stat(filepath.Join("web/static", filepath.FromSlash(key)))
	return err == nil
}

func main() {
	sheet := flag.String("sheet", "", "spreadsheet id")
	preferences := flag.String("preferences", "", "preferences spreadsheet id")
	flag.Parse()
	if *sheet == "" || *preferences == "" {
		log.Fatal("[ERROR] -sheet <spreadsheet id> and -preferences <spreadsheet id> are required")
	}
	source, err := data.NewSheet(map[string]string{"directory": *sheet, "preferences": *preferences})
	if err != nil {
		log.Fatalf("[ERROR] sheet source: %v", err)
	}
	model, err := directory.LoadModel(source, nil, staticFiles{})
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
	byStatus := map[directory.OptStatus]int{}
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
		byStatus[directory.OptIn], byStatus[directory.OptOut], byStatus[directory.OptDefault],
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
}
