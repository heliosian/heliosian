package mcp

import (
	"context"
	"fmt"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const maxOutput = 200 << 10

func (s *Server) addTools() {
	for _, t := range s.tools.Tools {
		tool := &sdk.Tool{Name: t.Name, Description: t.Description, InputSchema: t.Schema, Annotations: &sdk.ToolAnnotations{ReadOnlyHint: true}}
		s.server.AddTool(tool, func(ctx context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			m := s.deps.Data.Model()
			email := req.Extra.TokenInfo.UserID
			viewer := m.SignedIn(email)
			if viewer == "" {
				return nil, fmt.Errorf("%s is not in the directory", email)
			}
			answer, err := s.tools.Run(ctx, m, viewer, t.Name, req.Params.Arguments)
			if err != nil {
				return failed(err), nil
			}
			if len(answer.Text) > maxOutput {
				return failed(fmt.Errorf("the answer runs to %d bytes, past the %d this tool returns; narrow it, or add a limit", len(answer.Text), maxOutput)), nil
			}
			return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: answer.Text}}}, nil
		})
	}
}

func failed(err error) *sdk.CallToolResult {
	out := &sdk.CallToolResult{}
	out.SetError(err)
	return out
}
