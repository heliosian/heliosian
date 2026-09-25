package blob

import (
	"context"
	"fmt"
)

const MailBucket = "heliosian-mail"

type Archive struct {
	objects objects
}

func NewArchive(name string) (*Archive, error) {
	b, err := newBucket(name)
	if err != nil {
		return nil, err
	}
	return &Archive{objects: b}, nil
}

func (a *Archive) Get(ctx context.Context, name string) ([]byte, error) {
	o, err := a.objects.get(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	return o.data, nil
}

func (a *Archive) Put(ctx context.Context, name, mimeType string, content []byte) error {
	return a.objects.put(ctx, name, mimeType, content)
}
