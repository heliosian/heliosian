package access

type Viewer struct {
	Email     string
	Admin     bool
	Household map[string]bool
}

func (v Viewer) Mine(email string) bool {
	return email != "" && (email == v.Email || v.Household[email])
}
