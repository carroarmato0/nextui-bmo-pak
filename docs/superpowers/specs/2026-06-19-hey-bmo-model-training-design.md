# Hey BMO Wake-Word Model Training — Design

**Date:** 2026-06-19
**Status:** Approved design
**Depends on:** Phase 2 wake-word (shipped) — `internal/wakeword` detector, bundled base models, `assets/wakeword/`.
**Related (follow-up):** "Mod-provided wake words" (spec A) — lets a mod ship its own classifier; this spec defines the model contract it consumes.

## Goal

Produce BMO's own **"Hey BMO"** wake-word model (pronounced "Hey Beemo") to replace the stock `hey_jarvis` placeholder, via a **documented, reproducible pipeline anyone can run** — including future mod authors who want their own wake word. No GPU required of the follower (Colab path); a local-GPU path is also documented.

## Background

The on-device detector (`internal/wakeword`) runs the openWakeWord pipeline: a shared **melspectrogram** model → shared **speech-embedding** model → a small **wake-word classifier**. Only the classifier is wake-word-specific. Training therefore only needs to produce a new classifier ONNX with the contract the detector already expects:

- **Input:** `[1, 16, 96]` float32 — 16 consecutive openWakeWord embeddings (~1.28 s of context).
- **Output:** `[1, 1]` float32 — sigmoid wake score.
- Trained against the **same base mel + embedding models the pak bundles** (openWakeWord v0.5.1).

The base mel/embedding models are **not** retrained.

## Approach

A **pinned wrapper around openWakeWord's tooling**, committed in-repo, rather than re-deriving training (too much to maintain) or merely pointing at an upstream notebook that drifts (not reproducible for mod authors). The wrapper bakes in BMO's phrase config + naming + an in-repo Go evaluation harness, and runs identically on Colab (free GPU) or a local NVIDIA GPU.

## Deliverables & repository layout

```
training/wakeword/
  README.md              # step-by-step: Colab + local-GPU, prereqs, time/cost, the model contract
  hey-bmo-training.ipynb # the pinned training notebook (the pipeline below)
  requirements.txt       # pinned: openwakeword, piper-sample-generator, torch, audiomentations, onnx, numpy
  config.yaml            # phrase spellings, target sample counts, output name, threshold target
cmd/wakeword-eval/       # Go tool: score a candidate .onnx over positive/negative WAV folders
assets/wakeword/hey_bmo.onnx  # the committed, trained model (replaces the stock placeholder)
```

### Unit responsibilities

- **`training/wakeword/hey-bmo-training.ipynb`** — installs pinned deps, runs the pipeline, exports `hey_bmo.onnx`, prints held-out metrics. Self-contained; Colab- and local-GPU-runnable.
- **`training/wakeword/config.yaml`** — the only thing a follower edits to retarget a different phrase: phonetic spellings, positive/negative sample counts, augmentation knobs, output filename, target threshold. (Mod authors copy this and change the spellings.)
- **`training/wakeword/README.md`** — the human-facing guide; includes the **model contract** section.
- **`cmd/wakeword-eval/`** — depends only on `internal/wakeword` + the base models; takes `--lib`, `--mel`, `--emb`, `--model`, `--positives <dir>`, `--negatives <dir>`, `--threshold`; prints true-accept %, false-accepts (and per-hour estimate), and a suggested threshold; exits non-zero if ONNX I/O doesn't match the contract.

## Training pipeline (notebook steps)

1. **Synthesize positives** — piper-sample-generator produces N "Hey Beemo" utterances across many Piper voices, speaking rates, and pitches, from phonetic spellings (`Beemo`, `Bee-Moh`, `BEE-moh`). A small batch is auditioned first to confirm pronunciation.
2. **Negatives** — download openWakeWord's precomputed negative features + a held-out validation negative set.
3. **Augment** positives — RIR reverb, background noise, gain (openWakeWord's standard augmentation) to simulate room/device conditions.
4. **Compute features** — run positives/negatives through the **bundled base mel→embedding** models to get `[N,16,96]` feature tensors.
5. **Train** the classifier with openWakeWord's trainer; architecture = the `16×96 → 1` head the detector requires.
6. **Export** `hey_bmo.onnx` and print held-out accuracy + false-accepts/hour + a recommended threshold.

## Validation & threshold

- The notebook prints held-out **synthetic** accuracy + false-accepts/hour + recommended threshold.
- **`cmd/wakeword-eval`** reproduces an accept/false-accept check locally: it loads the candidate `.onnx` plus the bundled base models through `internal/wakeword`, runs over `--positives`/`--negatives` folders of 16 kHz mono WAVs, and reports true-accept %, false-accepts, and a suggested threshold. It also asserts the ONNX I/O matches the detector contract (`[1,16,96] → [1,1]`).
- **On-device test (documented):** copy the candidate to the deployed pak's `assets/wakeword/hey_bmo.onnx`, enable **Settings → WAKE WORD**, say "Hey Beemo", and watch `scripts/debug-logs.sh` for `wake word detected: score=…`; adjust the threshold if needed.

## Shipping the trained model

- The trained `hey_bmo.onnx` (~1.3 MB) is **committed** to `assets/wakeword/hey_bmo.onnx` — versioned with the code, no external host.
- **`scripts/fetch-wakeword-assets.sh`** changes to fetch only the **base, model-agnostic** assets (the onnxruntime aarch64 `.so` + `melspectrogram.onnx` + `embedding_model.onnx`); the classifier now comes from the committed repo file rather than the downloaded `hey_jarvis` placeholder.
- **`scripts/release.sh`** copies the committed `assets/wakeword/hey_bmo.onnx` into each pak (it already copies `assets/.`); the fetch step continues to supply the base models + `.so`.
- If `wakeword-eval` recommends a threshold other than 0.5, update the detector's default threshold constant (`internal/wakeword`) accordingly — a single value, informed by evaluation.

## Model contract (for mod authors / spec A)

A wake-word classifier is compatible iff:

- it is an ONNX model with input `[1, 16, 96]` float32 and output `[1, 1]` float32 (sigmoid score), and
- it was trained against openWakeWord's base **melspectrogram** + **embedding** models (v0.5.1) — the ones the pak bundles in `assets/wakeword/`.

The README documents this contract and the `config.yaml`-edit-then-run recipe so a mod author can produce their own model by changing only the phrase spellings.

## Testing

- **`cmd/wakeword-eval`** gets an env-gated unit test (skipped without the ORT lib/models, like the detector tests) using the **alexa** model + `alexa_test.wav`: asserts a high accept rate on the positive clip and ~0 accepts on silence, and that a mismatched-shape model is rejected.
- **On-device / manual:** per the validation section, before committing the final `hey_bmo.onnx`.

## Risks & mitigations

| Risk | Mitigation |
|------|------------|
| Piper mispronounces "Beemo" | Audition a small batch first (step 1); adjust phonetic spellings in `config.yaml` before the full run. |
| Colab GPU time / quota | README notes expected ~1–2 h and the negative-feature download size; local-GPU path documented as the alternative. |
| Large negative-feature download | README states the size; pipeline caches it. |
| Trained model needs a different threshold | `wakeword-eval` recommends one; detector default updated from a single constant. |
| Model drift from the bundled base | Contract pins openWakeWord v0.5.1 base models (already bundled); notebook installs the matching pinned `openwakeword`. |

## Out of scope

- Retraining the base mel/embedding models.
- The mod integration itself (manifest field, load-from-mod-FS) — that is spec A, a follow-up that consumes this spec's model contract.
- Continuous/online retraining or a hosted training service.
