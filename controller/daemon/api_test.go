package daemon

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/reef-pi/reef-pi/controller/settings"
	"github.com/reef-pi/reef-pi/controller/storage"
	"github.com/reef-pi/reef-pi/controller/telemetry"
	"github.com/reef-pi/reef-pi/controller/utils"
)

func TestAPI(t *testing.T) {
	store, err := storage.NewStore("api-test.db")
	defer store.Close()

	if err != nil {
		t.Fatal(err)
	}
	initializeSettings(store)
	s := settings.DefaultSettings
	s.Capabilities.DevMode = true
	if err := store.Update(Bucket, "settings", s); err != nil {
		t.Fatal(err)
	}
	store.Close()

	r, err := New("0.1", "api-test.db")
	if err != nil {
		t.Fatal("Failed to create new reef-pi controller. Error:", err)
	}
	r.settings.Capabilities.DevMode = true
	if err := r.Start(); err != nil {
		t.Fatal("Failed to load subsystem. Error:", err)
	}
	tr := utils.NewTestRouter()

	r.UnAuthenticatedAPI(tr.Router)
	r.AuthenticatedAPI(tr.Router)
	r.h.Check()
	if err := tr.Do("GET", "/api/health_stats", new(bytes.Buffer), nil); err != nil {
		t.Error("Failed to get per minute health data.Error:", err)
	}
	body := new(bytes.Buffer)
	json.NewEncoder(body).Encode(utils.Credentials{
		User:     "reef-pi",
		Password: "reef-pi",
	})
	if err := tr.Do("POST", "/api/credentials", body, nil); err != nil {
		t.Error("Failed to update creds via api")
	}
	if err := tr.Do("GET", "/api/settings", new(bytes.Buffer), nil); err != nil {
		t.Error("Failed to get settings via api")
	}
	body.Reset()
	json.NewEncoder(body).Encode(&settings.DefaultSettings)
	if err := tr.Do("POST", "/api/settings", body, nil); err != nil {
		t.Error("Failed to update settings via api")
	}
	if err := tr.Do("GET", "/api/settings", new(bytes.Buffer), nil); err != nil {
		t.Error("Failed to get settings via api")
	}
	body.Reset()
	json.NewEncoder(body).Encode(&telemetry.DefaultTelemetryConfig)
	if err := tr.Do("POST", "/api/telemetry", body, nil); err != nil {
		t.Error("Failed to update telemetry via api")
	}
	if err := tr.Do("GET", "/api/telemetry", new(bytes.Buffer), nil); err != nil {
		t.Fatal("Failed to get telemetry via api")
	}
	body.Reset()
	json.NewEncoder(body).Encode(&DefaultDashboard)
	if err := tr.Do("POST", "/api/dashboard", body, nil); err != nil {
		t.Error("Failed to update dashboard via api")
	}
	if err := tr.Do("GET", "/api/dashboard", new(bytes.Buffer), nil); err != nil {
		t.Error("Failed to get dashboard via api")
	}
	if err := r.LogError("test-error", "test message"); err != nil {
		t.Error(err)
	}
	if err := r.LogError("test-error-2", "test message"); err != nil {
		t.Error(err)
	}
	if err := tr.Do("GET", "/api/errors/test-error", new(bytes.Buffer), nil); err != nil {
		t.Error("Failed to list errors using api. Error:", err)
	}
	if err := tr.Do("DELETE", "/api/errors/test-error", new(bytes.Buffer), nil); err != nil {
		t.Error("Failed to delete individual error using api. Error:", err)
	}
	if err := tr.Do("GET", "/api/errors", new(bytes.Buffer), nil); err != nil {
		t.Error("Failed to list errors using api. Error:", err)
	}
	if err := tr.Do("DELETE", "/api/errors/clear", new(bytes.Buffer), nil); err != nil {
		t.Error("Failed to clear errors using api. Error:", err)
	}
	if err := tr.Do("POST", "/api/telemetry/test_message", new(bytes.Buffer), nil); err != nil {
		t.Error("Failed to send test message using api. Error:", err)
	}
	if err := r.Stop(); err != nil {
		t.Error(err)
	}
}
