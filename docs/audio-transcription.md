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

For compatible clients, `whisper-1`, `gpt-transcribe`, `gpt-4o-transcribe`, and `gpt-4o-mini-transcribe` are accepted as aliases. **ChatGPT chooses the actual speech model. These aliases do not select those public API models.**

Uploads, including multipart overhead, are limited to 26 MiB. Streaming, speaker diarization, timestamps, subtitle formats, prompts, and dictionary keywords are not supported. Unsupported fields return a clear HTTP 400 error before audio is sent upstream. Uploaded audio is excluded from request-body logs, including when full request logging is enabled.

```sh
curl http://YOUR_HOST:8317/v1/audio/transcriptions \
  -H "Authorization: Bearer $CLIPROXY_API_KEY" \
  -F model=codex-transcribe \
  -F file=@speech.wav \
  -F language=en
```

The JSON response is `{"text":"Your transcribed words."}`. The route uses OAuth credentials only; it does not fall back to an OpenAI API-key account.

## OpenWhispr

In Settings, choose AI Models, Speech-to-Text, then Dictation:

1. Choose Cloud Providers / Bring your own key, then Custom.
2. Set the endpoint to `http://YOUR_HOST:8317/v1`.
3. Enter your CLIProxyAPI client API key.
4. Set the model to `codex-transcribe`.
5. Leave custom dictionary prompts empty for this transcription route. A separately configured GPT cleanup model can still apply text edits after transcription.

Meetings and audio uploads have separate engine settings. Configure them independently if needed. The model alias is entered manually; the dynamic coding-model catalog does not advertise this backend-selected speech model.
