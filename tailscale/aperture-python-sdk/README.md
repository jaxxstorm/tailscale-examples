# Aperture Python SDK Example

Demonstrates calling both the Anthropic and OpenAI APIs through [Tailscale Aperture](https://tailscale.com/kb/1295/aperture) using their official Python SDKs. No API keys are needed — Aperture handles authentication and routing on your tailnet.

## Prerequisites

- A Tailscale tailnet with [Aperture enabled](https://tailscale.com/kb/1295/aperture)
- Python 3.9+
- The machine running this script must be connected to your tailnet

## Usage

```bash
pip install -r requirements.txt
python main.py
```

## How It Works

Aperture exposes an AI gateway at `http://ai` on your tailnet. The script points each SDK's base URL at this gateway instead of the provider's public API:

| SDK | Base URL | Equivalent curl |
|-----|----------|-----------------|
| Anthropic | `http://ai` | `curl http://ai/v1/messages` |
| OpenAI | `http://ai/v1` | `curl http://ai/v1/chat/completions` |

Both SDKs require an `api_key` parameter to be set, but Aperture handles authentication so the value is unused — we pass `"not-needed"` as a placeholder.
