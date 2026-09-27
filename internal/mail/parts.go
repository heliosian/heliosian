package mail

import (
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	netmail "net/mail"
	"strings"

	"golang.org/x/net/html/charset"
)

type Part struct {
	MediaType   string
	Disposition string
	Name        string
	Body        io.Reader
}

func Parts(msg *netmail.Message, each func(Part) error) error {
	h := msg.Header
	return walk(h.Get("Content-Type"), h.Get("Content-Transfer-Encoding"), h.Get("Content-Disposition"), msg.Body, each)
}

func walk(contentType, encoding, disposition string, body io.Reader, each func(Part) error) error {
	if contentType == "" {
		contentType = "text/plain"
	}
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return fmt.Errorf("content type %q: %w", contentType, err)
	}
	if strings.HasPrefix(mediaType, "multipart/") {
		reader := multipart.NewReader(body, params["boundary"])
		for {
			part, err := reader.NextRawPart()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
			if err := walk(part.Header.Get("Content-Type"), part.Header.Get("Content-Transfer-Encoding"), part.Header.Get("Content-Disposition"), part, each); err != nil {
				return err
			}
		}
	}
	kind, dispositionParams, _ := mime.ParseMediaType(disposition)
	name := dispositionParams["filename"]
	if name == "" {
		name = params["name"]
	}
	content := body
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "base64":
		content = base64.NewDecoder(base64.StdEncoding, content)
	case "quoted-printable":
		content = quotedprintable.NewReader(content)
	}
	if label := params["charset"]; label != "" && strings.HasPrefix(mediaType, "text/") {
		if content, err = charset.NewReaderLabel(label, content); err != nil {
			return fmt.Errorf("charset %q: %w", label, err)
		}
	}
	return each(Part{MediaType: mediaType, Disposition: kind, Name: name, Body: content})
}
