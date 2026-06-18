[← Modding guide](../MODDING.md) · Quotes

# Idle quotes (`quotes.txt`)

`quotes.txt` holds the pool of verbatim one-liners BMO may speak
spontaneously between conversations (when proactive talk is enabled in
`config.json`), or on demand when you press the **X** button.

Each non-blank, non-comment line is one quote, spoken exactly as written —
no paraphrasing, no LLM involvement. Rules:

- **One quote per line.**
- **Blank lines** are ignored.
- **Lines starting with `#`** are treated as comments and ignored.

BMO re-reads the file before picking each quote, so additions take effect
immediately without a restart.

When `quotes.txt` is absent or blank, BMO falls back to its built-in pool
of Adventure Time one-liners.

---

## Example

```
# Grumpy detective BMO quotes

I've seen things, kid. Things you wouldn't believe.
The rain never stops in this city. Neither do the questions.
Every case starts the same way. Someone loses something they shouldn't have had.
# TODO: add more noir quotes
Put down the controller and nobody gets hurt.
```

---

## Notes

- Quotes are chosen at random from the pool; there is no guaranteed order.
- Proactive quoting is off by default (`proactive_talk: "off"` in
  `config.json`). Set it to `occasional`, `regular`, `chatty`, or `rare`
  to enable it. The **X** button speaks a quote regardless of the
  `proactive_talk` setting, so you can test your pool without enabling
  proactive talk — it does require AI mode (a TTS voice must be configured).
- Quotes are spoken with the current TTS voice and speaking style — the
  same voice and `voice.txt` instructions as regular conversation.
