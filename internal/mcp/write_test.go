package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"heliosian/internal/auth"
	"heliosian/internal/testkit"
)

func TestMCPWritesUseTheApprovingPersonsPermissions(t *testing.T) {
	for _, endpoint := range []string{"/mcp", "/"} {
		t.Run(endpoint, func(t *testing.T) {
			f := setup(t)
			session := f.connectAt(t, f.token(t), endpoint)
			text, failed := callTool(t, session, "helios_write", map[string]any{"batch": []any{
				map[string]any{"set": rowanID, "cells": map[string]any{"name_long_override": "Rowan via MCP"}},
				map[string]any{"insert": "PERSON_SETTING", "as": "setting", "row": map[string]any{"person": rowanID, "app": "when", "key": "quiet", "value": true}},
				map[string]any{"set": "@setting", "cells": map[string]any{"value": "no"}},
			}})
			if failed {
				t.Fatalf("an authorized batch failed: %s", text)
			}
			var written struct {
				Result []string `json:"result"`
			}
			if err := json.Unmarshal([]byte(text), &written); err != nil || len(written.Result) != 3 || written.Result[0] != rowanID || written.Result[1] != written.Result[2] {
				t.Fatalf("the write answered %s: %v", text, err)
			}
			person, _ := f.data.Model().Table("PERSON").Get(rowanID)
			setting, _ := f.data.Model().Table("PERSON_SETTING").Get(written.Result[1])
			if person["name_show"] != "Rowan via MCP" || setting["value"] != "no" {
				t.Fatalf("the write left person %v and setting %v", person, setting)
			}
			text, failed = callTool(t, session, "helios_write", map[string]any{"batch": []any{map[string]any{"delete": written.Result[1]}}})
			if failed {
				t.Fatalf("removing one's own setting failed: %s", text)
			}
			if _, ok := f.data.Model().Table("PERSON_SETTING").Get(written.Result[1]); ok {
				t.Fatal("the deleted setting remains")
			}
			found := false
			for _, change := range testkit.Rows(t, f.dir, f.data, "datapeople", "CHANGES") {
				if change["row"] == rowanID && change["column"] == "name_long_override" {
					found = true
					if change["actor"] != rowan || change["real_actor"] != rowan {
						t.Errorf("the write was attributed to %v", change)
					}
				}
			}
			if !found {
				t.Fatal("the profile edit has no change record")
			}
		})
	}
}

func TestMCPWritesRollBackForbiddenBatches(t *testing.T) {
	f := setup(t)
	session := f.connect(t, f.token(t))
	before, _ := f.data.Model().Table("PERSON").Get(rowanID)
	for _, forbidden := range []map[string]any{
		{"set": "per00000020001", "cells": map[string]any{"name_long_override": "Someone else's name"}},
		{"set": rowanID, "cells": map[string]any{"vc_name": "Imported name"}},
		{"insert": "MEMBER", "row": map[string]any{"group": "grp00000000005", "person": rowanID, "member": "yes"}},
	} {
		text, failed := callTool(t, session, "helios_write", map[string]any{"batch": []any{
			map[string]any{"set": rowanID, "cells": map[string]any{"name_long_override": "Should roll back"}}, forbidden,
		}})
		if !failed || !strings.Contains(text, "you may not") {
			t.Fatalf("a forbidden batch answered %s, failed %v", text, failed)
		}
		after, _ := f.data.Model().Table("PERSON").Get(rowanID)
		if after["name_long_override"] != before["name_long_override"] || f.data.Model().SuperAdmin(rowan) {
			t.Fatalf("the refused batch left changes: %v", after)
		}
	}
}

func TestMCPWritesUseTheSharedBatchParser(t *testing.T) {
	f := setup(t)
	session := f.connect(t, f.token(t))
	for _, input := range []map[string]any{
		{"batch": []any{}},
		{"batch": []any{map[string]any{"set": rowanID, "cells": map[string]any{"name_long_override": "Wrong"}, "server": true}}},
		{"batch": []any{map[string]any{"set": rowanID, "cells": map[string]any{"name_long_override": "Wrong"}}}, "viewer": "per00000000003"},
	} {
		text, failed := callTool(t, session, "helios_write", input)
		if !failed {
			t.Fatalf("a malformed batch succeeded: %s", text)
		}
	}
	listed, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tool := range listed.Tools {
		if tool.Name == "helios_write" {
			found = true
			if tool.Annotations == nil || tool.Annotations.ReadOnlyHint || tool.Annotations.IdempotentHint {
				t.Errorf("write annotations are %v", tool.Annotations)
			}
		}
	}
	if !found {
		t.Fatal("no write tool advertised")
	}
}

func TestSigningOutRevokesMCPWrites(t *testing.T) {
	f := setup(t)
	session := f.connect(t, f.token(t))
	if err := f.sessions.SignOut(context.Background(), rowan); err != nil {
		t.Fatal(err)
	}
	result, err := session.CallTool(context.Background(), &sdk.CallToolParams{Name: "helios_write", Arguments: map[string]any{"batch": []any{
		map[string]any{"set": rowanID, "cells": map[string]any{"name_long_override": "After sign-out"}},
	}}})
	if err == nil && !result.IsError {
		t.Fatal("a signed-out token made an edit")
	}
	row, _ := f.data.Model().Table("PERSON").Get(rowanID)
	if row["name_long_override"] == "After sign-out" {
		t.Fatal("sign-out did not prevent the edit")
	}
}

func TestEveryApprovalDisclosesEditAccess(t *testing.T) {
	f := setup(t)
	client := f.register(t)
	body, _ := json.Marshal(map[string]string{"query": f.authorizeQuery(client, challengeOf(verifier))})
	if status, about := f.post(t, "/api/mcp/request", "application/json", string(body), map[string]string{"Sec-Fetch-Site": "same-origin"}); status != http.StatusOK || about["client"] != "Test Client" {
		t.Fatalf("the approval description answered %d: %v", status, about)
	}
	page, err := os.ReadFile("../../web/mcp/authorize.html")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page), "can change what you can change") || strings.Contains(string(page), "can change nothing") {
		t.Fatalf("the approval page misstates access: %s", page)
	}
}

func TestOldAccessTokensRequireFreshApproval(t *testing.T) {
	f := setup(t)
	old := auth.Seal([]byte("test"), "access", map[string]any{"e": rowan, "i": time.Now().Unix()})
	batch := `{"batch":[{"set":"` + rowanID + `","cells":{"name_long_override":"Old approval"}}]}`
	for _, request := range []struct{ method, path, body string }{
		{http.MethodPost, "/mcp", `{}`},
		{http.MethodPost, "/", `{}`},
		{"QUERY", "/api/q", `["from","PERSON"]`},
		{http.MethodPost, "/api/q", batch},
		{http.MethodPost, "/api/do/wiki", `{}`},
		{http.MethodGet, "/api/blob/cnt00000000001/blob", ""},
	} {
		req, err := http.NewRequest(request.method, f.server.URL+request.path, strings.NewReader(request.body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+old)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s accepted an old token: %d", request.method, request.path, res.StatusCode)
		}
	}
	fresh := f.token(t)
	if text, failed := callTool(t, f.connect(t, fresh), "helios_write", map[string]any{"batch": []any{
		map[string]any{"set": rowanID, "cells": map[string]any{"name_long_override": "Fresh approval"}},
	}}); failed {
		t.Fatalf("a fresh approval could not write through MCP: %s", text)
	}
	if status, out := f.post(t, "/api/q", "application/json", batch, map[string]string{"Authorization": "Bearer " + fresh}); status != http.StatusOK {
		t.Fatalf("a fresh approval could not write through the API: %d %v", status, out)
	}
}

func TestOldApprovalCodesCannotMintAccessTokens(t *testing.T) {
	f := setup(t)
	client := f.register(t)
	now := time.Now()
	old := auth.Seal([]byte("test"), "code", grant{Client: client, Redirect: redirect, Challenge: challengeOf(verifier), Email: rowan, Issued: now.Unix(), Expires: now.Add(codeLength).Unix()})
	if status, out := f.exchange(t, client, old, verifier); status != http.StatusBadRequest || out["error"] != "invalid_grant" {
		t.Fatalf("an old approval code minted access: %d %v", status, out)
	}
	if status, out := f.exchange(t, client, f.approve(t, client), verifier); status != http.StatusOK {
		t.Fatalf("the registered client could not reapprove: %d %v", status, out)
	}
}

func TestOldApprovalPagesMustReloadBeforeApproving(t *testing.T) {
	f := setup(t)
	client := f.register(t)
	body, _ := json.Marshal(map[string]string{"query": f.authorizeQuery(client, challengeOf(verifier))})
	if status, _ := f.post(t, "/api/mcp/approve", "application/json", string(body), map[string]string{"Sec-Fetch-Site": "same-origin"}); status != http.StatusBadRequest {
		t.Fatalf("an old approval page issued a new grant: %d", status)
	}
}
