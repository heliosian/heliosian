package blob

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	storage "google.golang.org/api/storage/v1"
)

const MailBucket = "heliosian-mail"

type Archive struct {
	service *storage.Service
	bucket  string
}

func NewArchive(bucket string) (*Archive, error) {
	service, err := storage.NewService(context.Background(), option.WithScopes(storage.DevstorageReadWriteScope))
	if err != nil {
		return nil, fmt.Errorf("storage client: %w", err)
	}
	return &Archive{service: service, bucket: bucket}, nil
}

func (a *Archive) Get(ctx context.Context, name string) ([]byte, error) {
	resp, err := a.service.Objects.Get(a.bucket, name).Context(ctx).Download()
	if err != nil {
		return nil, fmt.Errorf("read %s from %s: %w", name, a.bucket, err)
	}
	defer resp.Body.Close()
	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read %s from %s: %w", name, a.bucket, err)
	}
	return content, nil
}

func (a *Archive) Put(ctx context.Context, name, mimeType string, content []byte) error {
	_, err := a.service.Objects.Insert(a.bucket, &storage.Object{Name: name, ContentType: mimeType}).
		Media(bytes.NewReader(content), googleapi.ContentType(mimeType)).
		Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("write %s to %s: %w", name, a.bucket, err)
	}
	return nil
}
