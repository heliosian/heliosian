package db

import (
	"fmt"
	"time"
)

func SchoolYearStart(at time.Time) int {
	year := at.Year()
	if at.Month() < time.July {
		year--
	}
	return year
}

func SchoolYear(date string) string {
	at, err := time.ParseInLocation(DateLayout, date[:min(len(date), len(DateLayout))], School)
	if err != nil {
		return ""
	}
	year := SchoolYearStart(at)
	return fmt.Sprintf("%d-%d", year, year+1)
}
