package groups

import (
	"errors"
	"fmt"
	"testing"

	"google.golang.org/api/googleapi"
)

func TestNotFoundReadsTheLookupRefusal(t *testing.T) {
	missing := &googleapi.Error{Code: 403, Message: "Error(2028): Permission denied for resource tech@loop.heliosian.com (or it may not exist)."}
	if !notFound(fmt.Errorf("look up: %w", missing)) {
		t.Fatal("the missing-group 403 was not read as not found")
	}
	if !notFound(&googleapi.Error{Code: 404, Message: "Not found"}) {
		t.Fatal("a 404 was not read as not found")
	}
	if notFound(&googleapi.Error{Code: 403, Message: "The caller does not have permission"}) {
		t.Fatal("a real permission failure was read as not found")
	}
	if notFound(errors.New("network is down")) {
		t.Fatal("a plain error was read as not found")
	}
}
