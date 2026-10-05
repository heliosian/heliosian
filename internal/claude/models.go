package claude

import "github.com/anthropics/anthropic-sdk-go"

const (
	AskModel            = "claude-opus-5-5"
	AskEffort           = anthropic.BetaOutputConfigEffortMedium
	CalendarModel       = "claude-fable-5-1"
	CalendarEffort      = anthropic.OutputConfigEffortMax
	ClassifyModel       = "claude-sonnet-5-5"
	ClassifyEffort      = anthropic.OutputConfigEffortHigh
	ComposeModel        = "claude-opus-5-5"
	ComposeEffort       = anthropic.OutputConfigEffortMedium
	DescribeModel       = "claude-sonnet-5-5"
	DescribeEffort      = anthropic.OutputConfigEffortLow
	DigestModel         = "claude-sonnet-5-5"
	DigestEffort        = anthropic.OutputConfigEffortMedium
	SearchSummaryModel  = "claude-sonnet-5-5"
	SearchSummaryEffort = anthropic.OutputConfigEffortMedium
)
