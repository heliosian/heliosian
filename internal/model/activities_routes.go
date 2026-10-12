package model

import (
	"encoding/json"
	"net/http"
	"slices"

	"heliosian/internal/access"
	"heliosian/internal/blob"
	"heliosian/internal/mail"
)

type activitiesApp struct {
	store    *Store
	images   blob.Images
	calendar calendarApp
	mailer   *mail.Mailgun
}

type ActivitiesDeps struct {
	Store    *Store
	Images   blob.Images
	Calendar CalendarHooks
	Mailer   *mail.Mailgun
}

func NewActivities(d ActivitiesDeps) ActivitiesHooks {
	return ActivitiesHooks{app: activitiesApp{store: d.Store, images: d.Images, calendar: d.Calendar.app, mailer: d.Mailer}}
}

func (a activitiesApp) activities() *Activities {
	return a.store.Model().Activities
}

func (a activitiesApp) directory() *Directory {
	return a.store.Model().Directory
}

func (a activitiesApp) actor(r *http.Request) access.Actor {
	return a.store.Model().actor(r, "team")
}

type activityBody struct {
	ID                 string     `json:"id"`
	Year               string     `json:"year"`
	Title              string     `json:"title"`
	Parent             string     `json:"parent"`
	Category           string     `json:"category"`
	Status             string     `json:"status"`
	Description        string     `json:"description"`
	Image              string     `json:"image"`
	Flyer              string     `json:"flyer"`
	Highlight          *Highlight `json:"highlight"`
	Timing             string     `json:"timing"`
	Start              string     `json:"start"`
	End                string     `json:"end"`
	Location           string     `json:"location"`
	Spots              int        `json:"spots"`
	CoLeaderNeeded     bool       `json:"coLeaderNeeded"`
	VolunteersHidden   string     `json:"volunteersHidden"`
	VolunteersComplete bool       `json:"volunteersComplete"`
	DirectSignUp       string     `json:"directSignUp"`
	Priority           bool       `json:"priority"`
	SignUp             string     `json:"signUp"`
	PrettyID           string     `json:"prettyId"`
	AllowAdding        string     `json:"allowAdding"`
	TakeOver           bool       `json:"takeOver"`
}

type activityPatch map[string]json.RawMessage

func (p activityPatch) into(body *activityBody) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, body)
}

func (p activityPatch) fields() []string {
	out := []string{}
	for field := range p {
		if field != "id" {
			out = append(out, field)
		}
	}
	slices.Sort(out)
	return out
}

type activitySettingsBody struct {
	ExpenseFormURL string `json:"expenseFormUrl"`
	Intro          string `json:"intro"`
}

type notifyBody struct {
	Kinds []string `json:"kinds"`
}
