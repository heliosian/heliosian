package mail

import "context"

type RawSender interface {
	SendRaw(ctx context.Context, from string, to []string, raw []byte) error
}
