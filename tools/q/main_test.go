package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"heliosian/internal/blob"
	"heliosian/internal/data"
	"heliosian/internal/db"
	"heliosian/internal/qclient"
	"heliosian/internal/store"
	"heliosian/internal/testkit"
)

func sampleClient(t *testing.T) *qclient.Client {
	t.Helper()
	dir := &data.Dir{Root: "../../sampledata"}
	queue := store.NewQueue()
	s, err := db.NewStore(dir, dir, queue, db.NewSearchIndex())
	if err != nil {
		t.Fatal(err)
	}
	tokens := testkit.Tokens(func(email string) bool { return s.Model().SignedIn(email) != "" })
	mux := http.NewServeMux()
	db.Register(mux, s, queue, db.NewPictures(s, queue, blob.NewMemoryBucket()), tokens, time.Now)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &qclient.Client{Base: srv.URL, Token: tokens.Issue("maya.lindqvist@example.org"), Mode: qclient.ImportMode}
}

func TestReadShowsTables(t *testing.T) {
	c := sampleClient(t)
	a, err := c.QueryText(`(from MEMBER (where (= group "grp00000000020")) (include person))`)
	if err != nil {
		t.Fatal(err)
	}
	out := &strings.Builder{}
	show(out, a, nil)
	for _, want := range []string{"MEMBER: 2\n", "PERSON: 2\n", "id  ", "Juni (Juniper) Ashdown", "Rowan Ashdown"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out.String(), "pronunciation") {
		t.Errorf("a column no row fills is shown:\n%s", out)
	}

	a, err = c.Query(json.RawMessage(`{"from": "EFFECTIVE_MEMBER", "where": [{"=": [{"path": "group"}, "grp00000000020"]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	out.Reset()
	show(out, a, nil)
	if !strings.Contains(out.String(), "EFFECTIVE_MEMBER: 2\n") {
		t.Errorf("generated rows:\n%s", out)
	}
}

func TestReadShowsChosenColumnsWhole(t *testing.T) {
	c := sampleClient(t)
	a, err := c.QueryText(`(from PERSON (where (= id "per00000000001")))`)
	if err != nil {
		t.Fatal(err)
	}
	long := strings.Repeat("y", cellWidth+10)
	a.Resources["PERSON"]["per00000000001"]["vc_bio"] = long
	out := &strings.Builder{}
	show(out, a, []string{"id", "vc_bio"})
	if !strings.Contains(out.String(), long) {
		t.Errorf("the chosen column is cut short:\n%s", out)
	}
	if strings.Contains(out.String(), "name_long") {
		t.Errorf("a column not chosen is shown:\n%s", out)
	}
}

func TestWrite(t *testing.T) {
	c := sampleClient(t)
	ids, err := c.Write(json.RawMessage(`{"batch": [{"set": "per00000000001", "cells": {"vc_name": "June Ashdown"}}]}`))
	if err != nil || len(ids) != 1 || ids[0] != "per00000000001" {
		t.Fatalf("write: %v %v", ids, err)
	}
}

func TestCellsAreShortAndOneLine(t *testing.T) {
	if got := cell("line one\nline two"); got != "line one⏎line two" {
		t.Errorf("cell = %q", got)
	}
	if got := cell(strings.Repeat("x", 100)); len([]rune(got)) != cellWidth || !strings.HasSuffix(got, "…") {
		t.Errorf("cell = %q", got)
	}
}
