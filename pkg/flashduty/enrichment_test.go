package flashduty

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

// TestAlertPreview_CarriesDataSourceFields asserts the preview struct now
// carries data_source_type/data_source_name; AI-SRE depends on both to route
// /monit/query/rows calls.
func TestAlertPreview_CarriesDataSourceFields(t *testing.T) {
	ap := AlertPreview{
		AlertID:        "a1",
		Title:          "CPU high",
		Severity:       "Critical",
		Status:         "Triggered",
		StartTime:      1775912219,
		DataSourceType: "prometheus",
		DataSourceName: "prom-10.99.1.107",
		Labels:         map[string]string{"resource": "web-server-01"},
	}
	if ap.DataSourceType != "prometheus" {
		t.Fatalf("DataSourceType lost: %q", ap.DataSourceType)
	}
	if ap.DataSourceName != "prom-10.99.1.107" {
		t.Fatalf("DataSourceName lost: %q", ap.DataSourceName)
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

// TestFetchIncidentAlerts_PreservesDataSourceFields asserts that when the
// upstream /incident/alert/list response carries data_source_type /
// data_source_name, fetchIncidentAlerts's projection to AlertPreview
// keeps both — they are the two fields AI-SRE depends on.
func TestFetchIncidentAlerts_PreservesDataSourceFields(t *testing.T) {
	upstream := `{
      "data": {
        "total": 1,
        "items": [{
          "alert_id": "a1",
          "title": "CPU high",
          "severity": "Critical",
          "status": "Triggered",
          "trigger_time": 1775912219,
          "data_source_type": "prometheus",
          "data_source_name": "prom-10.99.1.107",
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
	if alerts[0].DataSourceType != "prometheus" {
		t.Fatalf("DataSourceType=%q, want prometheus", alerts[0].DataSourceType)
	}
	if alerts[0].DataSourceName != "prom-10.99.1.107" {
		t.Fatalf("DataSourceName=%q, want prom-10.99.1.107", alerts[0].DataSourceName)
	}
}
