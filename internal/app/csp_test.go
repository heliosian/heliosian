package app

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCSPReportDoesNotLogBearerURLs(t *testing.T) {
	var out bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&out, nil)))
	defer slog.SetDefault(previous)

	body := `{"csp-report":{"document-uri":"https://when.heliosian.com/ext/page-secret?token=query-secret","effective-directive":"script-src","blocked-uri":"https://elsewhere.example/script.js?key=third-secret","source-file":"https://when.heliosian.com/ext/page-secret"}}`
	r := httptest.NewRequest(http.MethodPost, reportPath, strings.NewReader(body))
	w := httptest.NewRecorder()
	report(w, r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d", w.Code)
	}
	for _, secret := range []string{"page-secret", "query-secret", "third-secret"} {
		if bytes.Contains(out.Bytes(), []byte(secret)) {
			t.Errorf("%s leaked into CSP log: %s", secret, out.String())
		}
	}
	if !bytes.Contains(out.Bytes(), []byte(`"document":"https://when.heliosian.com"`)) {
		t.Errorf("document origin missing from CSP log: %s", out.String())
	}
}
