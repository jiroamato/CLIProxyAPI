package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTranscriptionAudioIsNotCapturedWithFullRequestLogging(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		request := httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", strings.NewReader("private-audio"))
		request.Header.Set("Content-Type", "multipart/form-data; boundary=test")
		if shouldCaptureRequestBody(enabled, request) {
			t.Fatalf("audio captured with full logging=%v", enabled)
		}
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"input":"text"}`))
	if !shouldCaptureRequestBody(true, request) {
		t.Fatal("ordinary request logging changed")
	}
}

func TestTranscriptionAudioIsNotCapturedInErrorOnlyMode(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", strings.NewReader("private-audio"))
	request.Header.Set("Content-Type", "audio/wav")
	capture := attachDeferredRequestBodyCapture(request, nil, &RequestInfo{}, false, false)
	if capture == nil || !capture.skipCapture {
		t.Fatal("audio body is eligible for error-only capture")
	}
}
