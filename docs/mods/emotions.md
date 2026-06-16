[← Modding guide](../MODDING.md) · Emotions

# Emotion vocabulary (`mod.json` → `emotions`)

The `emotions` map in `mod.json` lets you add human-readable descriptions to
emotion names. These descriptions are included in the system prompt so the
LLM understands what each expression means and can choose appropriately.

```json
{
  "emotions": {
    "grumpy": "sulky and irritable",
    "ecstatic": "overjoyed and bouncing"
  }
}
```

---

## How emotions work

When BMO replies, the LLM may begin the text with a bracketed directive such
as `[happy]`. BMO strips the directive before speaking and shows the
corresponding face. The emotion vocabulary is the complete list of names the
LLM is allowed to emit.

The vocabulary is built from three sources, in order:

1. **Built-in names** (from the embedded face set) — every canonical face
   that is not a functional face (blink, listening, thinking, speaking,
   sleeping are functional and are never advertised to the LLM).
2. **Mod's on-disk faces** — the base names of `.svg` files in the mod's
   `faces/` directory, excluding functional names.
3. **Manifest descriptions** — the `emotions` map. A key that matches a
   name from sources 1 or 2 gets its description attached to that entry.
   A key that introduces a brand-new name adds that name to the vocabulary
   (useful if you want to describe an existing face differently or add a
   named alias).

Descriptions appear in the system prompt as `name — description` pairs,
helping the LLM pick the most fitting expression.

---

## Relationship to faces

For an emotion to display correctly, its name should have a matching face
file in the mod's `faces/` directory (or in the built-in set for overlay
mods). An emotion name with no matching face folds to `neutral`.

See [faces.md](./faces.md) for the full list of built-in expressions and
SVG format requirements.

---

## Example: grumpy detective mod

```json
{
  "name": "Detective BMO",
  "emotions": {
    "grumpy":   "sulky and irritable, done with everything",
    "skeptical": "raising one eyebrow, not buying it",
    "resigned":  "accepting the worst with a tired sigh"
  }
}
```

The names `grumpy`, `skeptical` match built-in expression files. `resigned`
would need a matching `resigned.svg` in the mod's `faces/` directory to
display as anything other than neutral.

---

## Notes

- Emotion names are case-insensitive; they are normalized to lowercase.
- The `emotions` map is optional — omit it entirely if you do not need
  custom descriptions or new names.
- Most mods do not need this field at all; the built-in emotion set covers
  the common expressions.
