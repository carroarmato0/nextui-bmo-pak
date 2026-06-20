---
name: deploy
description: Use when deploying the built BMO Pak (and/or example mods) to connected TrimUI handhelds over ADB. Detects every connected device, names it (e.g. "TrimUI Brick (tg5040)"), pushes the correct per-device build, and deploys to all detected devices by default. Use when the user says "deploy BMO", "push to the device(s)", "install on the Brick / Smart Pro", or "deploy the mods".
---

# BMO Pak — Deploy to Device(s)

`scripts/deploy.sh` (the pak) and `scripts/deploy-mods.sh` (example mods) push
over ADB to **every connected, recognized handheld by default**. Both source
`scripts/lib/adb-devices.sh`, which detects and labels devices.

**Key insight:** the device-tree model is the generic SoC (`sun50iw10`) on every
tg5040 unit, so it cannot tell a Brick from a Smart Pro. Detection therefore
uses the **framebuffer width** (1024 → Brick, 1280 → Smart Pro; tg5050 → Smart
Pro S) on top of the same platform signals the pak's `launch.sh` uses. Platform
is resolved **per device**, so a Brick and a Smart Pro plugged in together each
receive their own platform's build in a single run.

## Prerequisites

- `adb` installed; devices listed by `adb devices` in the `device` state
  (ADB enabled in NextUI Settings → Developer).
- A current build present at `dist/all/Tools/<platform>/BMO.pak`. Build it first
  with `./scripts/release.sh` if missing — `deploy.sh` warns and skips any device
  whose platform build is absent.

## Deploy workflow

1. Ensure a build exists (`ls dist/all/Tools/*/BMO.pak`); run
   `./scripts/release.sh` if not.
2. Deploy the pak:
   - `./scripts/deploy.sh`                 — all detected devices (default).
   - `./scripts/deploy.sh --device brick`  — one device, by serial or a substring
     of its label (`brick`, `smart pro`, a serial prefix, …).
   - `./scripts/deploy.sh --all`           — all devices, skipping the menu.
3. Deploy the example mods the same way: `./scripts/deploy-mods.sh [--all|--device …]`
   (selectable on-device under Settings → MOD).
4. Verify: each device prints a `==> <label> [<serial>] -> <dest>` header and the
   run ends with `Done. Deployed to N device(s).`

## Selection behavior

- **Default = all** detected supported devices.
- `--device <serial|name>`: exact serial, or case-insensitive substring of the
  label. An ambiguous match (e.g. `--device trimui` with two units) errors and
  lists the candidates — narrow it.
- `--all`: force all, skip the prompt.
- Run in a terminal with several devices and no flag → an interactive menu
  (`a` = all, default on Enter). Run non-interactively (from this skill / CI)
  with several devices and no flag → **all**, no prompt.
- Devices that are connected but not recognized as a BMO handheld are listed on
  stderr and skipped.

## SD-card mode (no device)

Pass a mounted SD path to copy instead of pushing over ADB. This mode has no
device to probe, so set the platform explicitly:

```sh
DEPLOY_PLATFORM=tg5040 ./scripts/deploy.sh /run/media/me/SDCARD
DEPLOY_PLATFORM=tg5040 ./scripts/deploy-mods.sh /run/media/me/SDCARD
```

`DEPLOY_PLATFORM` also force-overrides per-device detection in ADB mode (rarely
needed; normally let detection choose).

## Gotchas

- **`adb` consumes stdin inside a `while read` loop.** Both scripts feed the
  device list on FD 3 (`done 3<<EOF`) and redirect each adb call `</dev/null`.
  Without this, only the first device deploys and the rest leak to stdout.
  Preserve this pattern when editing the loops.
- A frozen / overheating device makes its push hang (slow SD writes). That is a
  device fault, not a script bug — stop the push and power-cycle the unit; don't
  keep probing it over ADB.
- On-device targets: pak → `/mnt/SDCARD/Tools/<platform>/BMO.pak`; mods →
  `/mnt/SDCARD/.userdata/<platform>/BMO/mods`.

## Self-test

`sh scripts/lib/adb-devices-test.sh` checks the pure mapping functions
(`model_to_platform`, `device_name`) with no device attached. Run it after
editing the detection logic.
