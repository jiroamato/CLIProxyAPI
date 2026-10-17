# Audio transcription through Codex OAuth

`POST /v1/audio/transcriptions` accepts an audio file and returns its transcript using an existing ChatGPT/Codex OAuth login. It reuses the proxy's OAuth selector and Codex HTTP transport. A separate OpenAI Platform API key is not required.

This route adapts the multipart request to `https://chatgpt.com/backend-api/transcribe`, following the approach used by [Cockpit Tools](https://github.com/jlcodes99/cockpit-tools/blob/main/sidecars/cockpit-cliproxy/audio_runtime.go). This is a ChatGPT backend endpoint, not the public OpenAI transcription API. Its availability depends on the signed-in account and upstream behavior.

## Request

Authenticate with a regular proxy client key and send `multipart/form-data`:

| Field | Supported values |
| --- | --- |
| `file` | Exactly one nonempty audio file |
| `model` | `codex-transcribe`, recommended |
| `language` | Optional language code, for example `en` |
| `response_format` | `json`, default, or `text` |
| `stream` | Omit, or set to `false` |
| `prompt` | Accepted as an optional compatibility hint; not applied by this speech backend |

For compatible clients, `whisper-1`, `gpt-transcribe`, `gpt-4o-transcribe`, and `gpt-4o-mini-transcribe` are accepted as aliases. **ChatGPT chooses the actual speech model. These aliases do not select those public API models.**

Uploads, including multipart overhead, are limited to 26 MiB. The file route does not stream. Speaker diarization, timestamps, subtitle formats, and dictionary `keywords[]` are not supported. Unsupported fields return a clear HTTP 400 error before audio is sent upstream. Uploaded audio is excluded from request-body logs, including when full request logging is enabled.

OpenWhispr automatically sends its dictionary and assistant name as a `prompt` hint. The route accepts this field so it does not prevent transcription, but omits it from the ChatGPT speech request and diagnostics. A nonempty hint produces the response header `X-CLIProxy-Transcription-Prompt: ignored`. Apply dictionary corrections through a separately configured text-cleanup model if needed.

```sh
curl http://YOUR_HOST:8317/v1/audio/transcriptions \
  -H "Authorization: Bearer $CLIPROXY_API_KEY" \
  -F model=codex-transcribe \
  -F file=@speech.wav \
  -F language=en
```

The JSON response is `{"text":"Your transcribed words."}`. The route uses OAuth credentials only; it does not fall back to an OpenAI API-key account.

## Live dictation

`GET /v1/audio/transcriptions/stream` upgrades to a WebSocket using the same proxy client authentication as the file route. It forwards dictation to `wss://chatgpt.com/backend-api/dictation/stream?dictation_surface=composer`. One Codex OAuth credential is selected through the runtime scheduler and remains pinned until the session ends. OAuth tokens never reach the client.

Send JSON text frames in this order:

1. `{"type":"session.start","dictation_session_id":"unique-id","attempt_id":"unique-attempt","config":{"input_audio_format":"pcm16","sample_rate_hz":16000,"num_channels":1,"language":"en"}}`. Omit `language` for automatic detection.
2. Wait for `session.started`, then send `{"type":"audio.append","audio":"<base64 mono PCM16 little-endian>"}` frames.
3. When capture stops, flush the remaining audio and send `{"type":"session.close"}` once.

The proxy requests segment delivery. `transcript.segment` and `transcript.final` contain `utterance_id`, `revision`, and `text`. Replace an utterance's earlier revision instead of appending each event. Ignore older revisions. Commit the final aggregate only after `session.updated` reports `session.status` as `closed` and all utterances have final text.

Cancellation closes the socket without `session.close`. A refused connection, disconnect, incomplete final, or finalization timeout should use the complete retained recording through the file route. Discard preview text before that upload so the same words are never delivered twice.

The client-facing model remains `codex-transcribe`; the optional `model` query parameter accepts that alias only. No model or dictionary prompt is sent upstream. Audio frames, transcripts, authentication protocols, and upstream error details are excluded from diagnostics. The authenticated desktop handshake and protocol were checked against the installed desktop code; availability still depends on the upstream account and service.

## OpenWhispr

In Settings, choose AI Models, Speech-to-Text, then Dictation:

1. Choose Cloud Providers / Bring your own key, then Custom.
2. Set the endpoint to `http://YOUR_HOST:8317/v1`.
3. Enter your CLIProxyAPI client API key.
4. Set the model to `codex-transcribe`. Our OpenWhispr fork uses live dictation for this custom model, displays partial text while recording, and falls back to a file upload if streaming is unavailable.
5. Dictionary entries can stay in OpenWhispr. Their speech hint is accepted but ignored by this route; a separately configured GPT cleanup model can apply dictionary corrections after transcription.

Meetings and audio uploads have separate engine settings. Configure them independently if needed. The standard `/v1/models` catalog advertises `codex-transcribe` when a non-disabled Codex OAuth login is registered, so OpenWhispr's connection test can validate it. The specialized Codex coding-model catalog remains separate.
