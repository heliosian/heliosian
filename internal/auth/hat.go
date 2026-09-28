package auth

import "net/http"

const HatCookie = "heliosian-super-edit"

func Hat(r *http.Request) bool {
	cookie, err := r.Cookie(HatCookie)
	return err == nil && cookie.Value == "1"
}
