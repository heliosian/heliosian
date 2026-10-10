package ops

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	cloudbuild "google.golang.org/api/cloudbuild/v1"
	run "google.golang.org/api/run/v2"

	"heliosian/internal/blob"
	"heliosian/internal/feedback"
)

const (
	project     = "heliosian"
	region      = "us-west1"
	service     = "heliosian"
	shownItems  = 12
	readBuilds  = 30
	measureAge  = 10 * time.Minute
	costHost    = "https://api.anthropic.com"
	costVersion = "2023-06-01"
)

type Measurable interface {
	Usage(ctx context.Context) (map[string]blob.Usage, error)
}

type Deps struct {
	GitHub   *feedback.GitHubApp
	Builds   *cloudbuild.Service
	Run      *run.Service
	AdminKey string
	Buckets  map[string]Measurable
}

type Commit struct {
	SHA     string    `json:"sha"`
	Message string    `json:"message"`
	Author  string    `json:"author"`
	Time    time.Time `json:"time"`
	URL     string    `json:"url"`
}

type Build struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	SHA      string `json:"sha"`
	Created  string `json:"created"`
	Started  string `json:"started"`
	Finished string `json:"finished"`
	LogURL   string `json:"logUrl"`
	Digest   string `json:"digest"`
}

type Serving struct {
	Revision string `json:"revision"`
	Image    string `json:"image"`
	Digest   string `json:"digest"`
	Created  string `json:"created"`
}

type Issue struct {
	Number  int       `json:"number"`
	Title   string    `json:"title"`
	URL     string    `json:"url"`
	Created time.Time `json:"created"`
	Labels  []string  `json:"labels"`
}

type Issues struct {
	Open  int     `json:"open"`
	Items []Issue `json:"items"`
}

type SpendDay struct {
	Date    string             `json:"date"`
	ByModel map[string]float64 `json:"byModel"`
}

type Spend struct {
	Days []SpendDay `json:"days"`
}

type Bucket struct {
	Name    string                         `json:"name"`
	Reading Reading[map[string]blob.Usage] `json:"reading"`
}

type Snapshot struct {
	Commits Reading[[]Commit] `json:"commits"`
	Builds  Reading[[]Build]  `json:"builds"`
	Serving Reading[Serving]  `json:"serving"`
	Issues  Reading[Issues]   `json:"issues"`
	Spend   Reading[Spend]    `json:"spend"`
	Buckets []Bucket          `json:"buckets"`
}

type Board struct {
	commits *Source[[]Commit]
	builds  *Source[[]Build]
	serving *Source[Serving]
	issues  *Source[Issues]
	spend   *Source[Spend]
	names   []string
	buckets map[string]*Source[map[string]blob.Usage]
}

func New(d Deps) *Board {
	b := &Board{
		commits: newSource("commits", []Commit{}, d.recentCommits),
		builds:  newSource("builds", []Build{}, d.recentBuilds),
		serving: newSource("serving", Serving{}, d.servingRevision),
		issues:  newSource("issues", Issues{Items: []Issue{}}, d.openIssues),
		spend:   newSource("spend", Spend{Days: []SpendDay{}}, d.monthSpend),
		names:   slices.Sorted(maps.Keys(d.Buckets)),
		buckets: map[string]*Source[map[string]blob.Usage]{},
	}
	for name, m := range d.Buckets {
		b.buckets[name] = newSource("bucket "+name, map[string]blob.Usage{}, m.Usage)
	}
	for _, s := range []interface{ Refresh() }{b.commits, b.builds, b.serving, b.issues} {
		s.Refresh()
	}
	return b
}

func (b *Board) Freshen() {
	b.commits.retry()
	b.builds.retry()
	b.serving.retry()
	b.issues.retry()
	b.spend.freshen(measureAge)
	for _, s := range b.buckets {
		s.freshen(measureAge)
	}
}

func (b *Board) Snapshot() Snapshot {
	out := Snapshot{Commits: b.commits.Read(), Builds: b.builds.Read(), Serving: b.serving.Read(), Issues: b.issues.Read(), Spend: b.spend.Read(), Buckets: []Bucket{}}
	for _, name := range b.names {
		out.Buckets = append(out.Buckets, Bucket{Name: name, Reading: b.buckets[name].Read()})
	}
	return out
}

func (d Deps) recentCommits(ctx context.Context) ([]Commit, error) {
	var found []struct {
		SHA    string `json:"sha"`
		URL    string `json:"html_url"`
		Commit struct {
			Message string `json:"message"`
			Author  struct {
				Name string    `json:"name"`
				Date time.Time `json:"date"`
			} `json:"author"`
		} `json:"commit"`
	}
	if err := d.GitHub.Read(ctx, fmt.Sprintf("/repos/%s/commits?sha=main&per_page=%d", feedback.Repo, shownItems), &found); err != nil {
		return nil, err
	}
	out := []Commit{}
	for _, c := range found {
		first, _, _ := strings.Cut(c.Commit.Message, "\n")
		out = append(out, Commit{SHA: c.SHA, Message: first, Author: c.Commit.Author.Name, Time: c.Commit.Author.Date, URL: c.URL})
	}
	return out, nil
}

func (d Deps) openIssues(ctx context.Context) (Issues, error) {
	var found struct {
		Total int `json:"total_count"`
		Items []struct {
			Number  int       `json:"number"`
			Title   string    `json:"title"`
			URL     string    `json:"html_url"`
			Created time.Time `json:"created_at"`
			Labels  []struct {
				Name string `json:"name"`
			} `json:"labels"`
		} `json:"items"`
	}
	q := url.QueryEscape("repo:" + feedback.Repo + " is:issue is:open")
	if err := d.GitHub.Read(ctx, fmt.Sprintf("/search/issues?q=%s&sort=created&order=desc&per_page=%d", q, shownItems), &found); err != nil {
		return Issues{}, err
	}
	out := Issues{Open: found.Total, Items: []Issue{}}
	for _, i := range found.Items {
		labels := []string{}
		for _, l := range i.Labels {
			labels = append(labels, l.Name)
		}
		out.Items = append(out.Items, Issue{Number: i.Number, Title: i.Title, URL: i.URL, Created: i.Created, Labels: labels})
	}
	return out, nil
}

func (d Deps) recentBuilds(ctx context.Context) ([]Build, error) {
	found, err := d.Builds.Projects.Locations.Builds.List(fmt.Sprintf("projects/%s/locations/%s", project, region)).PageSize(readBuilds).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("list builds: %w", err)
	}
	out := []Build{}
	for _, b := range found.Builds {
		build := Build{ID: b.Id, Status: b.Status, SHA: b.Substitutions["COMMIT_SHA"], Created: b.CreateTime, Started: b.StartTime, Finished: b.FinishTime, LogURL: b.LogUrl}
		if b.Results != nil && len(b.Results.Images) > 0 {
			build.Digest = b.Results.Images[0].Digest
		}
		out = append(out, build)
	}
	return out, nil
}

func (d Deps) servingRevision(ctx context.Context) (Serving, error) {
	svc, err := d.Run.Projects.Locations.Services.Get(fmt.Sprintf("projects/%s/locations/%s/services/%s", project, region, service)).Context(ctx).Do()
	if err != nil {
		return Serving{}, fmt.Errorf("read the service: %w", err)
	}
	rev, err := d.Run.Projects.Locations.Services.Revisions.Get(svc.LatestReadyRevision).Context(ctx).Do()
	if err != nil {
		return Serving{}, fmt.Errorf("read revision %s: %w", svc.LatestReadyRevision, err)
	}
	if len(rev.Containers) == 0 {
		return Serving{}, fmt.Errorf("revision %s has no container", rev.Name)
	}
	image := rev.Containers[0].Image
	_, digest, ok := strings.Cut(image, "@")
	if !ok {
		return Serving{}, fmt.Errorf("revision %s runs %s, not pinned to a digest", rev.Name, image)
	}
	parts := strings.Split(rev.Name, "/")
	return Serving{Revision: parts[len(parts)-1], Image: image, Digest: digest, Created: rev.CreateTime}, nil
}

func (d Deps) monthSpend(ctx context.Context) (Spend, error) {
	now := time.Now().UTC()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	out := Spend{Days: []SpendDay{}}
	page := ""
	for {
		q := url.Values{"starting_at": {start.Format(time.RFC3339)}, "group_by[]": {"description"}, "limit": {"31"}}
		if page != "" {
			q.Set("page", page)
		}
		var found struct {
			Data []struct {
				Start   time.Time `json:"starting_at"`
				Results []struct {
					Amount string `json:"amount"`
					Model  string `json:"model"`
					Desc   string `json:"description"`
				} `json:"results"`
			} `json:"data"`
			More bool   `json:"has_more"`
			Next string `json:"next_page"`
		}
		if err := d.adminGet(ctx, "/v1/organizations/cost_report?"+q.Encode(), &found); err != nil {
			return Spend{}, err
		}
		for _, day := range found.Data {
			sd := SpendDay{Date: day.Start.Format(time.DateOnly), ByModel: map[string]float64{}}
			for _, r := range day.Results {
				cents, err := strconv.ParseFloat(r.Amount, 64)
				if err != nil {
					return Spend{}, fmt.Errorf("cost report amount %q: %w", r.Amount, err)
				}
				model := r.Model
				if model == "" {
					model = r.Desc
				}
				sd.ByModel[model] += cents / 100
			}
			out.Days = append(out.Days, sd)
		}
		if !found.More {
			return out, nil
		}
		page = found.Next
	}
}

func (d Deps) adminGet(ctx context.Context, path string, into any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, costHost+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("x-api-key", d.AdminKey)
	req.Header.Set("anthropic-version", costVersion)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("anthropic admin %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		reply, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return fmt.Errorf("anthropic admin %s: %d: %s", path, resp.StatusCode, strings.TrimSpace(string(reply)))
	}
	return json.NewDecoder(resp.Body).Decode(into)
}
