# Self-Hosted Speech (faster-whisper + Piper via Speaches)

BMO's chat, speech-to-text (STT), and text-to-speech (TTS) each speak the
OpenAI REST API and are configured **independently** in `config.json`. That
means you can point STT and TTS at a self-hosted, OpenAI-compatible server
instead of OpenAI.

The recommended server is **[Speaches](https://github.com/speaches-ai/speaches)**
(the maintained successor to faster-whisper-server). One Speaches instance
serves both:

- **STT** via `POST /v1/audio/transcriptions`, backed by **faster-whisper**.
- **TTS** via `POST /v1/audio/speech`, backed by **Piper** (or Kokoro).

> Note: plain LLM servers such as Ollama or LM Studio implement only
> `/v1/chat/completions` and `/v1/models` — they do **not** serve audio
> endpoints. Use them for `chat` only, and Speaches (or OpenAI) for STT/TTS.

## Run Speaches

CPU-only:

```bash
docker run --rm -p 8000:8000 \
  -e WHISPER__MODEL=Systran/faster-whisper-small \
  ghcr.io/speaches-ai/speaches:latest-cpu
```

For GPU, use the `latest-cuda` image. See the Speaches docs for model and
voice selection.

## Configure BMO

In `config.json`, set the `base_url` of the STT and TTS providers to your
Speaches instance (`api_key` may be any non-empty string; Speaches ignores it
unless you enabled auth). `chat` can stay on OpenAI, an LLM box, or Speaches.

```json
{
  "stt": {
    "active": "speaches",
    "providers": [
      {
        "name": "speaches",
        "model": "Systran/faster-whisper-small",
        "base_url": "http://192.168.50.90:8000/v1",
        "api_key": "local"
      }
    ]
  },
  "tts": {
    "active": "speaches",
    "providers": [
      {
        "name": "speaches",
        "model": "speaches-ai/piper-en_US-amy-low",
        "voice": "amy",
        "base_url": "http://192.168.50.90:8000/v1",
        "api_key": "local"
      }
    ]
  }
}
```

BMO requests TTS as WAV and reads the sample rate from the response header, so
Piper voices (commonly 22050 Hz) play at the correct pitch with no extra
configuration.
