"""
Demonstrates calling the Anthropic and OpenAI APIs through Tailscale Aperture
using the official Python SDKs.

Aperture provides an AI gateway on your tailnet at http://ai, so no API keys
are needed — authentication and routing are handled by Tailscale.
"""

import argparse

import anthropic
import openai


APERTURE_BASE_URL = "http://ai"

DEFAULT_ANTHROPIC_MODEL = "claude-haiku-4-5-20251001"
DEFAULT_OPENAI_MODEL = "gpt-5.1"
DEFAULT_PROMPT = "respond with: hello"
DEFAULT_MAX_TOKENS = 25


def call_anthropic(prompt: str, model: str, max_tokens: int):
    """Call the Anthropic API through Aperture using the official SDK."""
    # The Anthropic SDK appends /v1/messages to the base_url automatically,
    # so we just point it at the Aperture gateway.
    client = anthropic.Anthropic(
        base_url=APERTURE_BASE_URL,
        # No API key needed — Aperture handles auth.
        # The SDK requires the field to be set, so we pass a placeholder.
        api_key="not-needed",
    )

    message = client.messages.create(
        model=model,
        max_tokens=max_tokens,
        messages=[{"role": "user", "content": prompt}],
    )

    print("=== Anthropic (via Aperture) ===")
    print(f"Model: {message.model}")
    print(f"Response: {message.content[0].text}")
    print()


def call_openai(prompt: str, model: str, max_tokens: int):
    """Call the OpenAI API through Aperture using the official SDK."""
    # The OpenAI SDK appends resource paths (e.g. /chat/completions) to
    # base_url, so we set it to http://ai/v1.
    client = openai.OpenAI(
        base_url=f"{APERTURE_BASE_URL}/v1",
        # No API key needed — Aperture handles auth.
        # The SDK requires the field to be set, so we pass a placeholder.
        api_key="not-needed",
    )

    completion = client.chat.completions.create(
        model=model,
        max_completion_tokens=max_tokens,
        messages=[{"role": "user", "content": prompt}],
    )

    print("=== OpenAI (via Aperture) ===")
    print(f"Model: {completion.model}")
    print(f"Response: {completion.choices[0].message.content}")
    print()


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Call LLM APIs through Tailscale Aperture.",
    )
    parser.add_argument(
        "--provider",
        choices=["anthropic", "openai", "both"],
        default="both",
        help="Which provider to call (default: both)",
    )
    parser.add_argument(
        "--prompt",
        default=DEFAULT_PROMPT,
        help=f"The prompt to send (default: '{DEFAULT_PROMPT}')",
    )
    parser.add_argument(
        "--anthropic-model",
        default=DEFAULT_ANTHROPIC_MODEL,
        help=f"Anthropic model to use (default: {DEFAULT_ANTHROPIC_MODEL})",
    )
    parser.add_argument(
        "--openai-model",
        default=DEFAULT_OPENAI_MODEL,
        help=f"OpenAI model to use (default: {DEFAULT_OPENAI_MODEL})",
    )
    parser.add_argument(
        "--max-tokens",
        type=int,
        default=DEFAULT_MAX_TOKENS,
        help=f"Maximum tokens in the response (default: {DEFAULT_MAX_TOKENS})",
    )
    return parser.parse_args()


if __name__ == "__main__":
    args = parse_args()

    if args.provider in ("anthropic", "both"):
        call_anthropic(args.prompt, args.anthropic_model, args.max_tokens)
    if args.provider in ("openai", "both"):
        call_openai(args.prompt, args.openai_model, args.max_tokens)
