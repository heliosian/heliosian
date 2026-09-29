package model

type overrideBody struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Start       string   `json:"start"`
	End         string   `json:"end"`
	Location    string   `json:"location"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Keywords    []string `json:"keywords"`
	Note        *string  `json:"note"`
	Address     string   `json:"address"`
}

type overrideImageBody struct {
	ID    string `json:"id"`
	Image string `json:"image"`
}
