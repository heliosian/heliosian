package admins

type Approval struct {
	App   string `json:"app"`
	Title string `json:"title"`
	Start string `json:"start,omitempty"`
	Path  string `json:"path"`
}
