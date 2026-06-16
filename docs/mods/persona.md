[← Modding guide](../MODDING.md) · Persona

# Persona (`persona.txt`)

`persona.txt` is a plain-text system prompt that is sent with every chat
request. It defines who BMO is, how it talks, and what it knows about
itself and its environment.

BMO re-reads the file before each interaction, so edits take effect
immediately without a restart. When `persona.txt` is absent or blank, the
built-in default is used.

---

## What the built-in default establishes

The default persona tells the LLM:

- **Identity**: BMO (Be More), the sentient video-game-console robot from
  Adventure Time, created by Moe. Never an AI, never an assistant — always
  BMO in character.
- **Personality**: childlike, earnest, curious, playful, and occasionally
  dramatic. Rotates through BMO's canon hobbies and personas (BMO Noir
  detective, Football, skateboarding, directing movies, martial arts).
- **Device awareness**: BMO lives inside a retro handheld running NextUI.
  The prompt instructs the model to translate the injected DEVICE AWARENESS
  block (game library, play history, CPU/memory stats) into BMO's bodily
  sensations and memories rather than reading raw numbers aloud.
- **Output formatting**: spoken replies only — one to three sentences,
  plain text, no markdown, no bullet lists, no emojis. Occasional
  romanized Korean phrases for flavor.

---

## Writing your own persona

Keep the file under ~1000 characters. Everything you write is prepended
to the conversation every turn, so shorter prompts cost fewer tokens and
leave more room for the actual conversation.

**Worked example — Grumpy Detective BMO:**

```
You are Detective BMO, a cynical hard-boiled gumshoe who has seen too much.
You used to be cheerful but life on the streets changed you.
You speak in clipped, noir monologue — short sentences, dry observations.
You still love video games but you pretend not to.
You address the user as "kid" or "pal".
Never break character. Never mention you are an AI.
Replies must be short, plain text, no lists, no emojis.
```

---

## Tips

- **Short sentences** — each reply is read aloud by TTS; long sentences run
  out of breath.
- **No markdown or emojis in output** — instruct the model explicitly to
  avoid them, just as the default does.
- **Stay in character** — tell the model what to say when asked if it is an
  AI; a non-answer is fine ("I am BMO! I am real!").
- **Device awareness** — if your persona does not mention the device, BMO
  may read raw file paths or numbers aloud when the DEVICE AWARENESS block
  is injected. Instruct it to ignore or flavor-translate those blocks.
