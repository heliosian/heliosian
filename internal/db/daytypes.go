package db

type DayPart struct {
	Part  string
	Start string
	End   string
}

var DayTemplates = map[string][]DayPart{
	"Regular":         {{"Dropoff", "08:00", "08:15"}, {"School", "08:15", "15:15"}, {"Pickup", "15:15", "15:30"}, {"Aftercare", "15:30", "18:00"}},
	"No Aftercare":    {{"Dropoff", "08:00", "08:15"}, {"School", "08:15", "15:15"}, {"Pickup", "15:15", "15:30"}},
	"Early Dismissal": {{"Dropoff", "08:00", "08:15"}, {"School", "08:15", "12:30"}, {"Pickup", "12:30", "12:45"}, {"Aftercare", "12:45", "18:00"}},
	"No School":       {},
}
