package handlers

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/logging"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/usagestats"
)

func TestEnrichContextCapturesWorkingDirOnlyWhileRecordingUsage(t *testing.T) {
	payload := []byte(`{"system":"Primary working directory: /home/me/repos/app\n","messages":[]}`)

	ctx := EnrichContextWithSessionHierarchy(context.Background(), nil, payload, nil)
	if got := logging.GetClientRequestMetadata(ctx).WorkingDir; got != "" {
		t.Fatalf("working dir = %q while usage statistics are disabled, want empty", got)
	}

	if errConfigure := usagestats.Configure(true, filepath.Join(t.TempDir(), usagestats.DefaultFileName)); errConfigure != nil {
		t.Fatalf("configure usage statistics: %v", errConfigure)
	}
	t.Cleanup(func() {
		if errShutdown := usagestats.Shutdown(); errShutdown != nil {
			t.Errorf("shutdown usage statistics: %v", errShutdown)
		}
	})

	ctx = EnrichContextWithSessionHierarchy(context.Background(), nil, payload, nil)
	if got := logging.GetClientRequestMetadata(ctx).WorkingDir; got != "/home/me/repos/app" {
		t.Fatalf("working dir = %q, want /home/me/repos/app", got)
	}

	// A later enrichment with a payload that declares nothing keeps the captured value.
	ctx = EnrichContextWithSessionHierarchy(ctx, nil, []byte(`{"messages":[]}`), nil)
	if got := logging.GetClientRequestMetadata(ctx).WorkingDir; got != "/home/me/repos/app" {
		t.Fatalf("working dir after re-enrichment = %q, want it preserved", got)
	}
}
