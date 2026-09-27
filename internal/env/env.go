package env

import (
	"encoding/json"
	"os"
	"strings"

	"heliosian/internal/logging"
)

func Required(name string) string {
	value := os.Getenv(name)
	if value == "" {
		logging.Fatal("environment variable is required", "name", name)
	}
	return value
}

func ClientID() string {
	if id := os.Getenv("GOOGLE_CLIENT_ID"); id != "" {
		return id
	}
	raw, err := os.ReadFile("local/creds/oauth-client.json")
	if err != nil {
		logging.Fatal("read local/creds/oauth-client.json (or set GOOGLE_CLIENT_ID)", "error", err)
	}
	var parsed struct {
		Web struct {
			ClientID string `json:"client_id"`
		} `json:"web"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil || parsed.Web.ClientID == "" {
		logging.Fatal("local/creds/oauth-client.json is not an oauth web client file")
	}
	return parsed.Web.ClientID
}

func Key(envName, file string) string {
	if key := os.Getenv(envName); key != "" {
		return key
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		logging.Fatal("read key file", "file", file, "or set", envName, "error", err)
	}
	key := strings.TrimSpace(string(raw))
	if key == "" {
		logging.Fatal("key file is empty", "file", file)
	}
	return key
}

func OptionalKey(envName, file string) string {
	if key := os.Getenv(envName); key != "" {
		return key
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}
