package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"heliosian/internal/access"
	"heliosian/internal/db"
)

const maxOutput = 200 << 10

func (s *Server) addTools() {
	for _, t := range s.deps.Tools.Tools {
		tool := &sdk.Tool{Name: t.Name, Description: t.Description, InputSchema: t.Schema, Annotations: &sdk.ToolAnnotations{ReadOnlyHint: true}}
		s.server.AddTool(tool, func(ctx context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			m := s.deps.Data.Model()
			email := req.Extra.TokenInfo.UserID
			viewer := m.SignedIn(email)
			if viewer == "" {
				return nil, fmt.Errorf("%s is not in the directory", email)
			}
			answer, err := s.deps.Tools.Run(ctx, m, viewer, t.Name, req.Params.Arguments)
			if err != nil {
				return failed(err), nil
			}
			if len(answer.Text) > maxOutput {
				return failed(fmt.Errorf("the answer runs to %d bytes, past the %d this tool returns; narrow it, or add a limit", len(answer.Text), maxOutput)), nil
			}
			return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: answer.Text}}}, nil
		})
	}
	schema, err := jsonschema.For[db.Batch](nil)
	if err != nil {
		panic(err)
	}
	s.server.AddTool(&sdk.Tool{
		Name:        "helios_write",
		Description: `Update Helios School's data as the person who connected: insert, set or delete rows in one atomic batch, with the same permissions as the apps. Read helios_describe_table and helios_policies before writing. Each edit is {"insert":"TABLE","as":"name","row":{"column":"value"}}, {"set":"row ID or @name","cells":{"column":"value"}}, or {"delete":"row ID or @name"}. IDs are minted by the server; later edits can reference an earlier insert with @name. Reference columns take row IDs. Cell values are text, numbers, booleans or null to clear. The result lists the affected IDs in edit order. A refused edit rolls back the whole batch. Writes can send the same mail and invitations as an edit in the apps.`,
		InputSchema: schema,
		Annotations: &sdk.ToolAnnotations{},
	}, s.write)
}

func (s *Server) write(ctx context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
	email := req.Extra.TokenInfo.UserID
	viewer := s.deps.Data.Model().SignedIn(email)
	if viewer == "" {
		return failed(fmt.Errorf("%s is not in the directory", email)), nil
	}
	batch, err := db.ParseBatch(req.Params.Arguments)
	if err != nil {
		return failed(err), nil
	}
	start := time.Now()
	ids, err := db.Write(ctx, s.deps.Data, access.Actor{Email: email}, db.Env{Viewer: viewer, Now: s.deps.Tokens.Now()}, batch)
	slog.InfoContext(ctx, "mcp: write", "viewer", viewer, "writes", len(batch.Batch), "took", time.Since(start), "error", err)
	if err != nil {
		return failed(err), nil
	}
	out, err := json.Marshal(struct {
		Result []string `json:"result"`
	}{Result: ids})
	if err != nil {
		return nil, err
	}
	return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: string(out)}}}, nil
}

func failed(err error) *sdk.CallToolResult {
	out := &sdk.CallToolResult{}
	out.SetError(err)
	return out
}
