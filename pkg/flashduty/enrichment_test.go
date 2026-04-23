package flashduty

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

// TestAlertPreview_CarriesIntegrationFields asserts the preview struct now
// carries integration_type/integration_name; AI-SRE depends on both to route
// /monit/query/rows calls.
func TestAlertPreview_CarriesIntegrationFields(t *testing.T) {
	ap := AlertPreview{
		AlertID:         "a1",
		Title:           "CPU high",
		Severity:        "Critical",
		Status:          "Triggered",
		StartTime:       1775912219,
		IntegrationType: "prometheus",
		IntegrationName: "prom-10.99.1.107",
		Labels:          map[string]string{"resource": "web-server-01"},
	}
	if ap.IntegrationType != "prometheus" {
		t.Fatalf("IntegrationType lost: %q", ap.IntegrationType)
	}
	if ap.IntegrationName != "prom-10.99.1.107" {
		t.Fatalf("IntegrationName lost: %q", ap.IntegrationName)
	}
}

// newTestClient builds a *Client pointed at baseURL for tests. The production
// NewClient rejects empty base URL and applies a real app_key check, so we
// construct the struct directly here — it's package-local.
func newTestClient(t *testing.T, baseURL string) *Client {
	t.Helper()
	parsed, err := url.Parse(baseURL)
	if err != nil {
		t.Fatalf("parse base URL: %v", err)
	}
	return &Client{
		httpClient: &http.Client{Timeout: 5 * time.Second},
		baseURL:    parsed,
		appKey:     "test-key",
		userAgent:  "flashduty-mcp-server-test",
	}
}

// TestFetchIncidentAlerts_PreservesIntegrationFields asserts that when the
// upstream /incident/alert/list response carries integration_type /
// integration_name, fetchIncidentAlerts's projection to AlertPreview
// keeps both — they are the two fields AI-SRE depends on.
func TestFetchIncidentAlerts_PreservesIntegrationFields(t *testing.T) {
	upstream := `{
      "data": {
        "total": 1,
        "items": [{
          "alert_id": "a1",
          "title": "CPU high",
          "severity": "Critical",
          "status": "Triggered",
          "trigger_time": 1775912219,
          "integration_type": "prometheus",
          "integration_name": "prom-10.99.1.107",
          "labels": {"resource":"web-server-01"}
        }]
      }
    }`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(upstream))
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)

	alerts, total, err := client.fetchIncidentAlerts(context.Background(), "inc-1", 5)
	if err != nil {
		t.Fatalf("fetchIncidentAlerts: %v", err)
	}
	if total != 1 || len(alerts) != 1 {
		t.Fatalf("unexpected alerts: total=%d len=%d", total, len(alerts))
	}
	if alerts[0].IntegrationType != "prometheus" {
		t.Fatalf("IntegrationType=%q, want prometheus", alerts[0].IntegrationType)
	}
	if alerts[0].IntegrationName != "prom-10.99.1.107" {
		t.Fatalf("IntegrationName=%q, want prom-10.99.1.107", alerts[0].IntegrationName)
	}
}
