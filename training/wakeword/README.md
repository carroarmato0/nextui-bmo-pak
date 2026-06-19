# Training a "Hey BMO" wake word

This directory holds a pinned, reproducible pipeline that produces BMO's
on-device wake-word classifier. Run it on **Google Colab (free GPU)** or a
**local NVIDIA GPU**. Anyone — including mod authors who want a different wake
phrase — can run it by editing one file (`config.yaml`).

## What you get

A single `hey_bmo.onnx` classifier (~1.3 MB) that plugs into BMO's existing
detector. The detector runs three models in series — a shared **melspectrogram**
model, a shared **embedding** model, and this **classifier**. Only the
classifier is phrase-specific; you do not retrain the base models.

## Model contract (read this if you're a mod author)

A classifier is compatible with BMO iff:

- it is an ONNX model with input `[1, 16, 96]` float32 (16 openWakeWord
  embeddings ≈ 1.28 s of audio) and output `[1, 1]` float32 (a sigmoid score), and
- it was trained against openWakeWord's **v0.5.1** base melspectrogram +
  embedding models — the exact ones the pak bundles in `assets/wakeword/`.

The repo tool `cmd/wakeword-eval` asserts this contract and is the gate before
shipping any model.

## Prerequisites

- A GPU. Colab's free tier is enough (expect ~1–2 h end to end). A local NVIDIA
  GPU works too.
- The pinned deps in `requirements.txt`.
- Disk/time for the negative-feature download (several GB; cached after the
  first run).

## Steps

1. Open `hey-bmo-training.ipynb` in Colab (Runtime → GPU) or Jupyter on your GPU box.
2. Run the **setup** cells: they install `requirements.txt`, clone
   piper-sample-generator at the pinned commit, and download the openWakeWord
   negative features, RIRs, and noise sets.
3. **Audition** (first synthesis cell): listen to ~10 generated "Hey Beemo"
   clips. If the pronunciation is wrong, edit `target_phrase` in `config.yaml`
   (e.g. `"hey bee-moh"`, `"hey BEE moh"`) and re-run the cell.
4. Run the **synthesize → augment → train** cells. They read `config.yaml`.
5. The notebook exports `hey_bmo.onnx` and prints held-out accuracy,
   false-accepts/hour, and a recommended threshold.
6. Download `hey_bmo.onnx`.

## Validate before shipping

From the repo root, with the base assets fetched (`./scripts/fetch-wakeword-assets.sh`):

```bash
CGO_ENABLED=1 go run ./cmd/wakeword-eval \
  -model /path/to/hey_bmo.onnx \
  -positives /path/to/positive_wavs \
  -negatives /path/to/negative_wavs \
  -threshold 0.5
```

`-positives` is a folder of 16 kHz mono WAVs of people saying "Hey BMO";
`-negatives` is unrelated speech/noise. The tool prints true-accept %,
false-accepts/hour, and a suggested threshold, and exits non-zero if the model
breaks the contract or the classes don't separate.

Then do an on-device check: copy the candidate to the deployed pak's
`assets/wakeword/hey_bmo.onnx`, enable **Settings → WAKE WORD**, say
"Hey Beemo", and watch `scripts/debug-logs.sh` for `wake word detected:
score=…`. Adjust the threshold if needed.

## Retargeting (mod authors)

Copy this directory, change `target_phrase` (and `model_name`) in `config.yaml`,
and run the notebook. The output ONNX obeys the same contract above, so it loads
into BMO unchanged.
