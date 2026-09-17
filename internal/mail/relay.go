package mail

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

type RawSender interface {
	SendRaw(ctx context.Context, from string, to []string, raw []byte) error
}

func (f *Files) SendRaw(ctx context.Context, from string, to []string, raw []byte) error {
	if err := os.MkdirAll(f.Dir, 0o755); err != nil {
		return err
	}
	name := filepath.Join(f.Dir, fmt.Sprintf("%s-%s.eml", time.Now().Format("20060102-150405.000000"), slug(to[0])))
	if err := os.WriteFile(name, raw, 0o644); err != nil {
		return err
	}
	slog.InfoContext(ctx, "mail: wrote raw message", "file", name, "from", from, "to", to, "bytes", len(raw))
	return nil
}
