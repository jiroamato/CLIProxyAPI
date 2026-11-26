package executor

import (
	"context"
	"io"
	"net/http"
	"testing"
	"testing/synctest"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/auth/copilot"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	coreusage "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/usage"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v8/sdk/translator"
)

// Each Copilot route must mark TTFT at the first token frame, as the Claude
// and Codex executors do, not at the first byte of the stream.
func TestCopilotExecutorStreamTTFTWaitsForFirstToken(t *testing.T) {
	tests := []struct {
		name       string
		model      string
		endpoint   string
		format     sdktranslator.Format
		payload    string
		preamble   string
		token      string
		completion string
	}{
		{
			name:     "messages",
			model:    "claude-opus-5",
			endpoint: copilot.Messages,
			format:   sdktranslator.FormatClaude,
			payload:  `{"model":"claude-opus-5","max_tokens":64,"stream":true,"messages":[{"role":"user","content":"hello"}]}`,
			preamble: "event: message_start\n" +
				`data: {"type":"message_start","message":{"id":"msg_ttft","type":"message","role":"assistant","model":"claude-opus-5","content":[],"usage":{"input_tokens":10,"output_tokens":1}}}` + "\n\n" +
				"event: ping\n" + `data: {"type":"ping"}` + "\n\n" +
				"event: content_block_start\n" + `data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}` + "\n\n",
			token: "event: content_block_delta\n" + `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}` + "\n\n",
			completion: "event: content_block_stop\n" + `data: {"type":"content_block_stop","index":0}` + "\n\n" +
				"event: message_delta\n" + `data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":5}}` + "\n\n" +
				"event: message_stop\n" + `data: {"type":"message_stop"}` + "\n\n",
		},
		{
			name:     "responses",
			model:    "gpt-6-sol",
			endpoint: copilot.Responses,
			format:   sdktranslator.FormatOpenAIResponse,
			payload:  `{"model":"gpt-6-sol","stream":true,"input":"hello"}`,
			preamble: "event: response.created\n" + `data: {"type":"response.created","response":{"id":"resp_ttft","object":"response","status":"in_progress","model":"gpt-6-sol","output":[]}}` + "\n\n" +
				"event: response.output_item.added\n" + `data: {"type":"response.output_item.added","output_index":0,"item":{"type":"message","id":"msg_ttft","role":"assistant","status":"in_progress","content":[]}}` + "\n\n",
			token: "event: response.output_text.delta\n" + `data: {"type":"response.output_text.delta","item_id":"msg_ttft","output_index":0,"content_index":0,"delta":"hello"}` + "\n\n",
			completion: "event: response.completed\n" +
				`data: {"type":"response.completed","response":{"id":"resp_ttft","object":"response","status":"completed","model":"gpt-6-sol","output":[{"type":"message","id":"msg_ttft","role":"assistant","status":"completed","content":[{"type":"output_text","text":"hello","annotations":[]}]}],"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}}}` + "\n\n",
		},
		{
			name:     "chat",
			model:    "gpt-6-sol",
			endpoint: copilot.Chat,
			format:   sdktranslator.FormatOpenAI,
			payload:  `{"model":"gpt-6-sol","stream":true,"messages":[{"role":"user","content":"hello"}]}`,
			preamble: `data: {"id":"chat_ttft","object":"chat.completion.chunk","model":"gpt-6-sol","choices":[{"index":0,"delta":{"role":"assistant"}}]}` + "\n\n",
			token:    `data: {"id":"chat_ttft","object":"chat.completion.chunk","model":"gpt-6-sol","choices":[{"index":0,"delta":{"content":"hello"}}]}` + "\n\n",
			completion: `data: {"id":"chat_ttft","object":"chat.completion.chunk","model":"gpt-6-sol","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}` + "\n\n" +
				"data: [DONE]\n\n",
		},
	}
	// Keep the global usage dispatcher outside the fake-clock bubble.
	coreusage.StartDefault(context.Background())
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			alias := t.Name()
			capture := &multiProviderUsageCapture{alias: alias, records: make(chan coreusage.Record, 4)}
			coreusage.RegisterNamedPlugin(alias, capture)
			t.Cleanup(func() {
				coreusage.RegisterNamedPlugin(alias, multiProviderNoopUsagePlugin{})
			})

			synctest.Test(t, func(t *testing.T) {
				transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
					_, _ = io.Copy(io.Discard, req.Body)
					// time.Sleep advances synctest's virtual clock, not wall time.
					time.Sleep(10 * time.Millisecond)
					reader, writer := io.Pipe()
					go func() {
						defer func() { _ = writer.Close() }()
						write := func(s string) bool {
							_, errWrite := io.WriteString(writer, s)
							return errWrite == nil
						}
						time.Sleep(90 * time.Millisecond)
						if !write(tt.preamble) {
							return
						}
						time.Sleep(900 * time.Millisecond)
						if !write(tt.token) {
							return
						}
						time.Sleep(100 * time.Millisecond)
						write(tt.completion)
					}()
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Type": {"text/event-stream"}},
						Body:       reader,
						Request:    req,
					}, nil
				})
				ctx := context.WithValue(context.Background(), "cliproxy.roundtripper", http.RoundTripper(transport))
				ctx = coreusage.WithRequestedModelAlias(ctx, alias)
				auth := copilotTestAuth(t, "copilot-ttft", tt.model, tt.endpoint)
				payload := []byte(tt.payload)
				result, errExecute := NewCopilotExecutor(&config.Config{}).ExecuteStream(ctx, auth,
					cliproxyexecutor.Request{Model: copilot.ModelPrefix + tt.model, Payload: payload},
					cliproxyexecutor.Options{SourceFormat: tt.format, OriginalRequest: payload, Stream: true})
				if errExecute != nil {
					t.Fatalf("ExecuteStream: %v", errExecute)
				}
				for chunk := range result.Chunks {
					if chunk.Err != nil {
						t.Fatalf("stream: %v", chunk.Err)
					}
				}
			})

			record := capture.await(t)
			if record.Failed {
				t.Fatalf("record failed: %+v", record)
			}
			if record.TTFT != time.Second {
				t.Fatalf("TTFT = %v, want 1s (first token frame, not the first byte)", record.TTFT)
			}
			if record.Latency != 1100*time.Millisecond {
				t.Fatalf("Latency = %v, want 1.1s", record.Latency)
			}
		})
	}
}
