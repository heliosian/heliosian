package data

import (
	"errors"
	"testing"

	"google.golang.org/api/googleapi"
)

func TestServerErrorsAreRetriedButNotForRemovals(t *testing.T) {
	quota := &googleapi.Error{Code: 429}
	down := &googleapi.Error{Code: 503}
	bad := &googleapi.Error{Code: 400}
	if !refused(quota) || refused(down) || refused(bad) {
		t.Fatal("only a 429 is refused")
	}
	if !unavailable(down) || unavailable(quota) || unavailable(bad) || unavailable(errors.New("plain")) {
		t.Fatal("only a 5xx is unavailable")
	}
	calls := 0
	_, err := callOnce("remove", func(...googleapi.CallOption) (int, error) {
		calls++
		return 0, down
	})
	if !errors.Is(err, down) || calls != 1 {
		t.Fatalf("a removal that met a server error was tried %d times: %v", calls, err)
	}
	calls = 0
	_, err = call("read", func(...googleapi.CallOption) (int, error) {
		calls++
		return 0, bad
	})
	if !errors.Is(err, bad) || calls != 1 {
		t.Fatalf("a bad request was tried %d times: %v", calls, err)
	}
}
