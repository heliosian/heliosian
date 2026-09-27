package mailtest

import (
	"context"
	"testing"
	"time"

	"heliosian/internal/mail"
)

type Discard struct{}

func (Discard) Send(context.Context, mail.Message) error { return nil }

type Recorder chan mail.Message

func NewRecorder() Recorder { return make(Recorder, 8) }

func (r Recorder) Send(_ context.Context, m mail.Message) error {
	r <- m
	return nil
}

func (r Recorder) Next(t *testing.T) mail.Message {
	t.Helper()
	select {
	case m := <-r:
		return m
	case <-time.After(2 * time.Second):
		t.Fatal("no mail arrived")
		return mail.Message{}
	}
}
