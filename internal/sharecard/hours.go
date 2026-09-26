package sharecard

import "time"

func Hours(start, end time.Time) string {
	if !end.After(start) {
		return start.Format("3:04 PM")
	}
	if start.Format("PM") == end.Format("PM") {
		return start.Format("3:04") + " – " + end.Format("3:04 PM")
	}
	return start.Format("3:04 PM") + " – " + end.Format("3:04 PM")
}
