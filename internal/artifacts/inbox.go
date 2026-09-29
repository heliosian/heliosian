package artifacts

import "heliosian/internal/blob"

type Inbox struct {
	SigningKey string
	Bucket     *blob.Bucket
}
