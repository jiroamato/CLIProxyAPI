package api

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/runtime/executor"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
)

// Capture function names and source locations only. Stack arguments, request
// headers, and credential values must never be included in verification output.
func transcriptionStackFrames() string {
	pcs := make([]uintptr, 80)
	n := runtime.Callers(2, pcs)
	frames := runtime.CallersFrames(pcs[:n])
	var result strings.Builder
	for {
		frame, more := frames.Next()
		fmt.Fprintf(&result, "%s\n\t%s:%d\n", frame.Function, frame.File, frame.Line)
		if !more {
			return result.String()
		}
	}
}

type transcriptionTraceWriter struct {
	http.ResponseWriter
	frames   chan string
	recorded bool
}

func (w *transcriptionTraceWriter) WriteHeader(status int) {
	if !w.recorded {
		w.recorded = true
		w.frames <- transcriptionStackFrames()
	}
	w.ResponseWriter.WriteHeader(status)
}

func TestTranscriptionHTTPStackTrace(t *testing.T) {
	const transcript = "Controlled network transcription."
	const upstreamToken = "synthetic-oauth-token-not-a-secret"
	audio := []byte("RIFF-synthetic-audio")
	for _, tc := range []struct {
		name       string
		key        string
		format     string
		upstream   int
		payload    string
		wantStatus int
		wantBody   string
	}{
		{"json", "test-key", "json", 200, `{"text":"` + transcript + `","private":"discard"}`, 200, `{"text":"` + transcript + `"}`},
		{"text", "test-key", "text", 200, `{"text":"` + transcript + `"}`, 200, transcript},
		{"upstream_unauthorized", "test-key", "json", 401, "<html>private upstream failure</html>", 401, "ChatGPT transcription failed (HTTP 401)"},
		{"upstream_forbidden", "test-key", "json", 403, "<html>private upstream failure</html>", 403, "ChatGPT transcription failed (HTTP 403)"},
		{"upstream_rate_limit", "test-key", "json", 429, "private upstream failure", 429, "ChatGPT transcription failed (HTTP 429)"},
		{"upstream_failure", "test-key", "json", 500, "private upstream failure", 500, "ChatGPT transcription failed (HTTP 500)"},
		{"malformed_upstream", "test-key", "json", 200, "<html>private upstream failure</html>", 502, "ChatGPT returned an invalid transcription response"},
		{"missing_client_key", "", "json", 0, "", 401, ""},
		{"incorrect_client_key", "invalid-test-key", "json", 0, "", 401, ""},
		{"unsupported_format", "test-key", "srt", 0, "", 400, "supports only json and text"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			upstreamValid := make(chan bool, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				valid := r.Method == http.MethodPost && r.URL.Path == "/backend-api/transcribe" &&
					r.Header.Get("Authorization") == "Bearer "+upstreamToken && r.Header.Get("Cookie") == ""
				reader, errMultipart := r.MultipartReader()
				fields := make(map[string]string)
				if errMultipart != nil {
					valid = false
				} else {
					for {
						part, errPart := reader.NextPart()
						if errPart == io.EOF {
							break
						}
						if errPart != nil {
							valid = false
							break
						}
						value, errRead := io.ReadAll(part)
						valid = valid && errRead == nil
						fields[part.FormName()] = string(value)
					}
				}
				valid = valid && len(fields) == 2 && fields["file"] == string(audio) && fields["language"] == "en"
				upstreamValid <- valid
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Set-Cookie", "private-upstream-cookie")
				w.Header().Set("Retry-After", "30")
				status := tc.upstream
				if status == 0 {
					status = http.StatusInternalServerError
				}
				w.WriteHeader(status)
				_, _ = io.WriteString(w, tc.payload)
			}))
			defer upstream.Close()
			server := newTestServer(t)
			server.handlers.AuthManager.RegisterExecutor(executor.NewCodexExecutor(server.cfg))
			_, errRegister := server.handlers.AuthManager.Register(context.Background(), &auth.Auth{
				ID: "synthetic-transcription-oauth", Provider: "codex", Status: auth.StatusActive,
				Attributes: map[string]string{"base_url": upstream.URL + "/backend-api/codex"},
				Metadata:   map[string]any{"access_token": upstreamToken},
			})
			if errRegister != nil {
				t.Fatal(errRegister)
			}
			transportFrames, responseFrames := make(chan string, 1), make(chan string, 1)
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				trace := &httptrace.ClientTrace{GotConn: func(httptrace.GotConnInfo) {
					select {
					case transportFrames <- transcriptionStackFrames():
					default:
					}
				}}
				r = r.WithContext(httptrace.WithClientTrace(r.Context(), trace))
				server.engine.ServeHTTP(&transcriptionTraceWriter{ResponseWriter: w, frames: responseFrames}, r)
			}))
			defer proxy.Close()
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			for name, value := range map[string]string{"model": "codex-transcribe", "language": "en", "response_format": tc.format, "prompt": "Synthetic dictionary term"} {
				if err := writer.WriteField(name, value); err != nil {
					t.Fatal(err)
				}
			}
			file, errFile := writer.CreateFormFile("file", "speech.wav")
			if errFile != nil {
				t.Fatal(errFile)
			}
			if _, err := file.Write(audio); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			request, errRequest := http.NewRequest(http.MethodPost, proxy.URL+"/v1/audio/transcriptions", &body)
			if errRequest != nil {
				t.Fatal(errRequest)
			}
			request.Header.Set("Content-Type", writer.FormDataContentType())
			request.Header.Set("Cookie", "private-client-cookie")
			if tc.key != "" {
				request.Header.Set("Authorization", "Bearer "+tc.key)
			}
			response, errResponse := proxy.Client().Do(request)
			if errResponse != nil {
				t.Fatal(errResponse)
			}
			payload, errRead := io.ReadAll(response.Body)
			if errClose := response.Body.Close(); errClose != nil {
				t.Fatal(errClose)
			}
			if errRead != nil || response.StatusCode != tc.wantStatus || !strings.Contains(string(payload), tc.wantBody) {
				t.Fatalf("HTTP result mismatch: status=%d readError=%v", response.StatusCode, errRead)
			}
			if tc.wantStatus == 200 && string(payload) != tc.wantBody {
				t.Fatal("unexpected fields in successful transcription")
			}
			if response.Header.Get("Set-Cookie") != "" || strings.Contains(string(payload), "private") {
				t.Fatal("upstream private data leaked to the client")
			}
			if len(responseFrames) == 0 {
				t.Fatal("response stack was not captured")
			}
			responseStack := <-responseFrames
			if !strings.Contains(responseStack, "AuthMiddleware") {
				t.Fatal("response stack did not traverse authentication middleware")
			}
			t.Logf("HTTP %d response stack:\n%s", response.StatusCode, responseStack)
			if tc.upstream == 0 {
				if calls.Load() != 0 || len(transportFrames) != 0 {
					t.Fatal("rejected request reached the upstream transport")
				}
				return
			}
			if calls.Load() != 1 || !<-upstreamValid {
				t.Fatal("upstream request changed audio or forwarded client credentials or compatibility fields")
			}
			if len(transportFrames) == 0 {
				t.Fatal("native upstream transport stack was not captured")
			}
			transportStack := <-transportFrames
			for _, frame := range []string{"(*CodexExecutor).HttpRequest", "(*Manager).HttpRequest", "(*Handler).HandleTranscription", "AuthMiddleware", "(*Engine).ServeHTTP"} {
				if !strings.Contains(transportStack, frame) {
					t.Fatalf("transport stack is missing %s", frame)
				}
			}
			t.Logf("Native upstream transport stack:\n%s", transportStack)
			if response.Header.Get("Retry-After") != "30" {
				t.Fatal("upstream retry guidance was lost")
			}
			if response.Header.Get("X-CLIProxy-Transcription-Prompt") != "ignored" {
				t.Fatal("ignored dictionary hint was not disclosed")
			}
		})
	}
}
