package model

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"

	"heliosian/internal/mail"
)

type CalendarMail struct {
	Sender     *mail.Mailgun
	ReplyTo    string
	Base       string
	SigningKey string
	Key        []byte
}

func (a calendarApp) replyToken(id, email string) string {
	mac := hmac.New(sha256.New, a.mail.Key)
	mac.Write([]byte(id + "|" + mail.Normalize(email)))
	return hex.EncodeToString(mac.Sum(nil)[:16])
}
