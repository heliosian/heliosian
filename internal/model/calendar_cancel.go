package model

type cancelBody struct {
	ID     string `json:"id"`
	Notify bool   `json:"notify"`
	Note   string `json:"note"`
}
