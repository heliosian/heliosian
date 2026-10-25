package authtest

import (
	"net/http"

	"heliosian/internal/auth"
)

func Fixed(email string, next http.Handler) http.Handler {
	admit := func(string) bool { return true }
	return auth.New("", "", nil, auth.Login{}, admit, nil, nil).Fixed(email, next)
}
