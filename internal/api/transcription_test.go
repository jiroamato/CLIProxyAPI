package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTranscriptionRouteAuthenticatesBeforeReadingAudio(t *testing.T) {
	server := newTestServer(t)
	for _, key := range []string{"", "wrong-key", "test-key"} {
		request := httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", strings.NewReader(`{}`))
		request.Header.Set("Content-Type", "application/json")
		if key != "" {
			request.Header.Set("Authorization", "Bearer "+key)
		}
		response := httptest.NewRecorder()
		server.engine.ServeHTTP(response, request)
		want := http.StatusUnauthorized
		if key == "test-key" {
			want = http.StatusBadRequest
		}
		if response.Code != want {
			t.Fatalf("key=%q status=%d want=%d body=%s", key, response.Code, want, response.Body)
		}
	}
}
