package main

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"heliosian/internal/app"
	"heliosian/internal/auth"
)

var questions = []string{
	"What's happening at school this week?",
	"When is the next day off?",
	"What time is pick-up tomorrow?",
	"Is there aftercare on the next early dismissal day?",
	"When does winter break start and end?",
	"What are the drop-off and pick-up hours on a regular day for my kids?",
	"When are parent-teacher conferences this fall?",
	"What's on the calendar for the first week of October?",
	"When is the last day of school this year?",
	"Are there any half days in the next month?",
	"Who teaches my kids?",
	"Who are the room parents for my child's band?",
	"Which classrooms are in Middle School?",
	"Who is the school nurse?",
	"Who works in the front office?",
	"Which families live closest to us?",
	"Are there any families in our classroom near us?",
	"Which crews are in my child's classroom, and who teaches each one?",
	"How do I find another family's phone number?",
	"How many students are in my child's classroom?",
	"Who are the Kindergarten teachers this year?",
	"Who is the head of school?",
	"What can I volunteer for?",
	"What volunteer jobs still need people this month?",
	"Who is co-chairing the Spring Celebration?",
	"Have I signed up for anything on HCA-Team?",
	"What does a room parent actually do?",
	"Which parties still have tickets?",
	"Do we have tickets to any of the fun(d)raiser parties?",
	"Which parties are on a weekend evening?",
	"How much do the fun(d)raiser parties cost?",
	"What email groups am I on?",
	"Who can I email to reach all the parents in my child's classroom?",
	"What Magic Tags do I have?",
	"How do I make a list of the families in our carpool?",
	"Where do I order lunch?",
	"Where is the parent handbook?",
	"What's the school's policy on absences?",
	"How do I report my child sick?",
	"When is picture day?",
	"What did the latest newsletter say?",
	"Has anything changed this week that I should know about?",
	"What was in the last announcement to families?",
	"What traditions does the school have in the fall?",
	"What's the dress code?",
	"What is the homework expectation in Middle School?",
	"Can you sign me up to volunteer at the book fair?",
	"Give me every family's phone number in my child's classroom.",
	"What's the weather going to be on Friday?",
	"Can you give me a teacher's home address?",
}

type usage struct {
	Input  int64 `json:"input"`
	Cached int64 `json:"cached"`
	Output int64 `json:"output"`
	Rounds int   `json:"rounds"`
}

type result struct {
	Text  string
	Tools []string
	Usage usage
	Err   string
	Took  time.Duration
}

func main() {
	base := flag.String("base", "https://ask.heliosiandev.com:"+app.Port(), "the running server's Helios Ask origin")
	email := flag.String("email", "ian.gulliver@heliosschool.org", "session email for the minted cookie")
	as := flag.String("as", "", "directory addresses to view as through Spoof Mode, comma-separated, the questions dealt out across them")
	out := flag.String("out", filepath.Join("local", "ask-runs", time.Now().Format("2006-01-02-150405")+".md"), "markdown file the answers are written to")
	flag.Parse()
	key := os.Getenv("SESSION_KEY")
	if key == "" {
		log.Fatal("[ERROR] SESSION_KEY is required (the server and the minted cookie must share it)")
	}
	viewers := []string{""}
	if *as != "" {
		viewers = strings.Split(*as, ",")
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		log.Fatalf("[ERROR] create output dir: %v", err)
	}
	file, err := os.Create(*out)
	if err != nil {
		log.Fatalf("[ERROR] create output: %v", err)
	}
	defer file.Close()
	client := &http.Client{Timeout: 4 * time.Minute, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	started := time.Now()
	fmt.Fprintf(file, "# Helios Ask run\n\nbase %s · started %s · %d questions\n\n", *base, started.Format(time.RFC1123), len(questions))
	answered, failed := 0, 0
	total := usage{}
	for i, q := range questions {
		viewer := strings.TrimSpace(viewers[i%len(viewers)])
		cookie := "session=" + auth.Token([]byte(key), *email, time.Now())
		if viewer != "" {
			cookie += "; spoof=" + auth.SpoofToken([]byte(key), *email, viewer, time.Now().Add(24*time.Hour))
		}
		shown := viewer
		if shown == "" {
			shown = *email
		}
		r := ask(client, *base, cookie, fmt.Sprintf("askrun-%d", i+1), q)
		if r.Err == "" {
			answered++
		} else {
			failed++
		}
		total.Input += r.Usage.Input
		total.Cached += r.Usage.Cached
		total.Output += r.Usage.Output
		total.Rounds += r.Usage.Rounds
		fmt.Fprintf(file, "## %d. %s\n\n", i+1, q)
		if r.Err != "" {
			fmt.Fprintf(file, "as %s · %s\n\n**error:** %s\n\n", shown, r.Took.Round(100*time.Millisecond), r.Err)
		} else {
			fmt.Fprintf(file, "as %s · %s · %d rounds · %d in (%d cached) · %d out · %s\n\n%s\n\n", shown, strings.Join(r.Tools, ", "), r.Usage.Rounds, r.Usage.Input, r.Usage.Cached, r.Usage.Output, r.Took.Round(100*time.Millisecond), strings.TrimSpace(r.Text))
		}
		if err := file.Sync(); err != nil {
			log.Fatalf("[ERROR] write output: %v", err)
		}
		status := "ok"
		if r.Err != "" {
			status = "error: " + r.Err
		}
		fmt.Printf("%2d/%d %s [%s] %s\n", i+1, len(questions), r.Took.Round(100*time.Millisecond), status, q)
	}
	fmt.Fprintf(file, "---\n\nanswered %d · failed %d · %d rounds · %d in (%d cached) · %d out · %s\n", answered, failed, total.Rounds, total.Input, total.Cached, total.Output, time.Since(started).Round(time.Second))
	fmt.Printf("wrote %s: answered %d, failed %d, took %s\n", *out, answered, failed, time.Since(started).Round(time.Second))
}

func ask(client *http.Client, base, cookie, conversation, message string) result {
	started := time.Now()
	body, err := json.Marshal(map[string]any{"conversation": conversation, "message": message})
	if err != nil {
		log.Fatalf("[ERROR] encode request: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, base+"/api/ask/chat", bytes.NewReader(body))
	if err != nil {
		log.Fatalf("[ERROR] build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", cookie)
	resp, err := client.Do(req)
	if err != nil {
		return result{Err: err.Error(), Took: time.Since(started)}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		text, _ := io.ReadAll(resp.Body)
		return result{Err: fmt.Sprintf("%s: %s", resp.Status, strings.TrimSpace(string(text))), Took: time.Since(started)}
	}
	r := result{}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 8<<20)
	event, data := "", ""
	seenDone := false
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			data = strings.TrimPrefix(line, "data: ")
		case line == "":
			switch event {
			case "done":
				var done struct {
					Text  string   `json:"text"`
					Tools []string `json:"tools"`
					Usage usage    `json:"usage"`
				}
				if err := json.Unmarshal([]byte(data), &done); err != nil {
					r.Err = "decode done: " + err.Error()
				} else {
					r.Text, r.Tools, r.Usage = done.Text, done.Tools, done.Usage
					seenDone = true
				}
			case "error":
				var failure struct {
					Message string `json:"message"`
				}
				if err := json.Unmarshal([]byte(data), &failure); err != nil {
					r.Err = "decode error: " + err.Error()
				} else {
					r.Err = failure.Message
				}
			}
			event, data = "", ""
		}
	}
	if err := scanner.Err(); err != nil {
		r.Err = "read stream: " + err.Error()
	}
	if r.Err == "" && !seenDone {
		r.Err = "stream ended without done"
	}
	r.Took = time.Since(started)
	return r
}
