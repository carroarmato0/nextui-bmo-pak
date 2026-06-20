# Training a "Hey BMO" wake word

This guide produces BMO's on-device wake-word classifier (`hey_bmo.onnx`) using
**openWakeWord's official, maintained training notebook** on Google Colab (free
GPU) or a local NVIDIA GPU. Anyone — including mod authors who want a different
wake phrase — can follow it by changing two values.

We intentionally do **not** ship our own training notebook: openWakeWord's
automatic trainer and its config schema change between versions, and a forked
copy drifts out of date quickly. Instead we pin openWakeWord's own notebook to a
known-good commit and layer our phrase on top. Our repo's job is to give you the
phrase overrides (`config.yaml`), validate the result (`cmd/wakeword-eval`), and
document the contract the model must satisfy.

## What you get

A single `hey_bmo.onnx` classifier (~1.3 MB) that plugs into BMO's existing
detector. The detector runs three models in series — a shared **melspectrogram**
model, a shared **embedding** model, and this **classifier**. Only the
classifier is phrase-specific; you do not retrain the base models. A classifier
trained by the openWakeWord pipeline below is compatible with the base models
the pak bundles because openWakeWord uses the same fixed melspectrogram +
embedding models across versions.

## Model contract (read this if you're a mod author)

A classifier is compatible with BMO iff:

- it is an ONNX model with input `[1, 16, 96]` float32 (16 openWakeWord
  embeddings ≈ 1.28 s of audio) and output `[1, 1]` float32 (a sigmoid score), and
- it was trained against openWakeWord's base melspectrogram + embedding models —
  the ones the pak bundles in `assets/wakeword/`.

The repo tool `cmd/wakeword-eval` asserts this contract and is the gate before
shipping any model. (This is what openWakeWord's automatic trainer produces by
default, so following the steps below yields a compatible model.)

## Prerequisites

- A GPU. Colab's free tier is enough. A local NVIDIA GPU works too.
- Automatic training is **Linux-only** (a Piper TTS constraint) — Colab is Linux,
  so the Colab path always works.
- Disk/time for openWakeWord's negative-feature + noise downloads (several GB,
  handled by the notebook's setup cells).

## Steps

1. Open openWakeWord's automatic training notebook **pinned to a known-good
   commit** directly in Colab (set Runtime → GPU):

   <https://colab.research.google.com/github/dscripka/openWakeWord/blob/368c03716d1e/notebooks/automatic_model_training.ipynb>

   (`368c03716d1e` is the commit this guide was validated against. Using `main`
   instead will usually work but may have drifted.)

2. Run the **Environment setup** cell as-is. It installs PyTorch, clones the
   `dscripka/piper-sample-generator` fork and downloads its voice model, and
   installs openWakeWord with training extras.

3. In the **"Modify values in the config"** cell, apply BMO's overrides from
   [`config.yaml`](config.yaml):

   ```python
   config["target_phrase"] = ["hey beemo"]   # optionally add "hey bee-moh"
   config["model_name"]    = "hey_bmo"
   config["n_samples"]     = 20000           # the demo uses 1000 only for speed
   config["n_samples_val"] = 2000
   config["steps"]         = 50000
   ```

   Mod authors: change `target_phrase` (and `model_name`) here to train a
   different wake word.

4. **Audition** the synthetic clips the notebook generates in Step 1. If "Hey
   BMO" sounds wrong, change the spelling (e.g. `"hey bee-moh"`, `"hey BEE moh"`)
   and regenerate.

5. Run the notebook's **generate → augment → train** steps. It exports the model
   to `<output_dir>/hey_bmo.onnx` and prints held-out accuracy and a
   false-positive estimate.

6. Download `hey_bmo.onnx`.

## Validate before shipping

From the repo root, with the base assets fetched (`./scripts/fetch-wakeword-assets.sh`),
run our evaluation tool on an aarch64 host (the device) or any host whose
`libonnxruntime.so` matches its CPU:

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
breaks the contract or the classes don't separate. (Convert clips with
`ffmpeg -i in.wav -ac 1 -ar 16000 out.wav`.)

Then do an on-device check: copy the candidate to the deployed pak's
`assets/wakeword/hey_bmo.onnx`, enable **Settings → WAKE WORD**, say
"Hey Beemo", and watch `scripts/debug-logs.sh` for `wake word detected:
score=…`. Adjust the threshold if needed.

## Retargeting (mod authors)

Change `target_phrase` (and `model_name`) in the config cell, run the notebook,
and validate. The output ONNX obeys the same contract above, so it loads into
BMO unchanged.
