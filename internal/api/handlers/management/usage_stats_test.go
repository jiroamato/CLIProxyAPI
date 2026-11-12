package management

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/usagestats"
)

func withUsageStatsStore(t *testing.T) *usagestats.Store {
	t.Helper()
	if errConfigure := usagestats.Configure(true, filepath.Join(t.TempDir(), usagestats.DefaultFileName)); errConfigure != nil {
		t.Fatalf("configure usage statistics: %v", errConfigure)
	}
	t.Cleanup(func() {
		if errShutdown := usagestats.Shutdown(); errShutdown != nil {
			t.Errorf("shutdown usage statistics: %v", errShutdown)
		}
	})
	return usagestats.Current()
}

func serveUsageStats(t *testing.T, handler gin.HandlerFunc, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(rec)
	ginCtx.Request = httptest.NewRequest(method, target, strings.NewReader(body))
	handler(ginCtx)
	return rec
}

func TestUsageStatsEndpointsReportDisabled(t *testing.T) {
	h := &Handler{}
	for name, handler := range map[string]gin.HandlerFunc{
		"stats": h.GetUsageStats, "speed": h.GetUsageSpeed, "settings": h.GetUsageSettings, "put settings": h.PutUsageSettings,
	} {
		rec := serveUsageStats(t, handler, http.MethodGet, "/v8/management/observability/usage/stats", "{}")
		if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "usage_statistics_disabled") {
			t.Errorf("%s: status = %d body = %s, want 409 usage_statistics_disabled", name, rec.Code, rec.Body.String())
		}
	}
}

func TestGetUsageStatsGroupsFiltersAndPrices(t *testing.T) {
	store := withUsageStatsStore(t)
	at := time.Date(2026, 11, 12, 6, 0, 0, 0, time.UTC)
	for _, row := range []usagestats.Row{
		{Timestamp: at, Model: "claude-opus-5-5", Project: "alpha", InputTokens: 1_000_000, TotalTokens: 1_000_000},
		{Timestamp: at, Model: "claude-opus-5-5", Project: "beta", InputTokens: 2_000_000, TotalTokens: 2_000_000},
	} {
		if errInsert := store.Insert(context.Background(), row); errInsert != nil {
			t.Fatalf("insert row: %v", errInsert)
		}
	}

	target := "/v8/management/observability/usage/stats?group_by=day,project&filter.project=beta&utc_offset_minutes=-480" +
		"&from=" + msString(at.Add(-time.Hour)) + "&to=" + msString(at.Add(time.Hour))
	rec := serveUsageStats(t, (&Handler{}).GetUsageStats, http.MethodGet, target, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Groups []struct {
			Dimensions  map[string]string `json:"dimensions"`
			TotalTokens int64             `json:"total_tokens"`
			CostUSD     float64           `json:"cost_usd"`
		} `json:"groups"`
	}
	if errDecode := json.Unmarshal(rec.Body.Bytes(), &payload); errDecode != nil {
		t.Fatalf("decode response: %v", errDecode)
	}
	if len(payload.Groups) != 1 {
		t.Fatalf("groups = %+v, want only the filtered project", payload.Groups)
	}
	group := payload.Groups[0]
	// 06:00 UTC is still the previous day at UTC-8.
	if group.Dimensions["day"] != "2026-11-11" || group.Dimensions["project"] != "beta" || group.TotalTokens != 2_000_000 || group.CostUSD != 8 {
		t.Fatalf("group = %+v", group)
	}
}

func TestGetUsageStatsRejectsBadQueries(t *testing.T) {
	withUsageStatsStore(t)
	for _, query := range []string{
		"group_by=nonsense", "filter.nonsense=x", "from=abc", "to=abc", "utc_offset_minutes=9000", "limit=0", "from=20&to=10",
	} {
		rec := serveUsageStats(t, (&Handler{}).GetUsageStats, http.MethodGet, "/v8/management/observability/usage/stats?"+query, "")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d body = %s, want 400", query, rec.Code, rec.Body.String())
		}
	}
	rec := serveUsageStats(t, (&Handler{}).GetUsageSpeed, http.MethodGet, "/v8/management/observability/usage/speed?group_by=day", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("speed by day: status = %d, want 400", rec.Code)
	}
}

func TestUsageSettingsRoundTripAndValidation(t *testing.T) {
	withUsageStatsStore(t)
	h := &Handler{}

	body := `{"model_prices":{"my-model":{"input":1.5,"output":6}},"account_plans":[{"provider":"claude","account":"me@example.com","monthly_usd":200}]}`
	if rec := serveUsageStats(t, h.PutUsageSettings, http.MethodPut, "/v8/management/observability/usage/settings", body); rec.Code != http.StatusOK {
		t.Fatalf("put status = %d body = %s", rec.Code, rec.Body.String())
	}

	rec := serveUsageStats(t, h.GetUsageSettings, http.MethodGet, "/v8/management/observability/usage/settings", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get status = %d body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Settings      usagestats.Settings         `json:"settings"`
		DefaultPrices map[string]usagestats.Price `json:"default_prices"`
	}
	if errDecode := json.Unmarshal(rec.Body.Bytes(), &payload); errDecode != nil {
		t.Fatalf("decode response: %v", errDecode)
	}
	if payload.Settings.ModelPrices["my-model"].Output != 6 || len(payload.Settings.AccountPlans) != 1 {
		t.Fatalf("settings = %+v", payload.Settings)
	}
	if len(payload.DefaultPrices) == 0 {
		t.Fatal("default prices missing from response")
	}

	for name, invalid := range map[string]string{
		"negative price": `{"model_prices":{"m":{"input":-1,"output":1}}}`,
		"unknown field":  `{"model_prices":{},"surprise":true}`,
		"malformed":      `{`,
	} {
		if rec := serveUsageStats(t, h.PutUsageSettings, http.MethodPut, "/v8/management/observability/usage/settings", invalid); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d body = %s, want 400", name, rec.Code, rec.Body.String())
		}
	}
}

func msString(t time.Time) string {
	raw, _ := json.Marshal(t.UnixMilli())
	return string(raw)
}
