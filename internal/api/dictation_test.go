package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestDictationStreamAuthenticatesAndCollectsSafeStack(t *testing.T) {
	server := newTestServer(t)
	for _, key := range []string{"", "wrong-key", "test-key"} {
		request := httptest.NewRequest(http.MethodGet, "/v1/audio/transcriptions/stream", nil)
		if key != "" {
			request.Header.Set("Authorization", "Bearer "+key)
		}
		recorder := httptest.NewRecorder()
		writer := &transcriptionTraceWriter{ResponseWriter: recorder, frames: make(chan string, 1)}
		server.engine.ServeHTTP(writer, request)
		want := http.StatusUnauthorized
		if key == "test-key" {
			want = http.StatusUpgradeRequired
		}
		if recorder.Code != want {
			t.Fatalf("status=%d want=%d", recorder.Code, want)
		}
		trace := <-writer.frames
		if strings.Contains(trace, "wrong-key") || strings.Contains(trace, "test-key") {
			t.Fatal("credential in stack trace")
		}
		if key == "test-key" {
			if !strings.Contains(trace, "HandleDictationStream") {
				t.Fatal("dictation handler missing from stack")
			}
			if path := os.Getenv("R4_DICTATION_STACK_OUTPUT"); path != "" {
				if os.WriteFile(path, []byte(trace), 0600) != nil {
					t.Fatal("unable to save sanitized stack")
				}
			}
		}
	}
}
