# Install

## Requirements

Linux needs a running PulseAudio or PipeWire (`pipewire-pulse`) server - the desktop default - for sound playback; macOS and Windows need nothing extra. `push-it doctor` cannot yet probe the audio server, so it will not tell you if this is missing.

## Quick start

```sh
curl -fsSL https://raw.githubusercontent.com/InfiniteRoomLabs/push-it/main/install.sh | sh -s -- --all
```

`install.sh` downloads the latest release for your OS/arch (Linux and macOS, amd64 and arm64), verifies its `checksums.txt` entry, puts `push-it` in `~/.local/bin`, and runs `push-it install "$@"` with whatever flags you passed. Env overrides: `PUSH_IT_VERSION` (default `latest`, or pin a tag like `v0.1.0`), `PUSH_IT_BIN_DIR` (default `~/.local/bin`).

Windows: grab the zip from the [releases page](https://github.com/InfiniteRoomLabs/push-it/releases) and run `push-it install`.

From source: `go install github.com/InfiniteRoomLabs/push-it/cmd/push-it@latest` then `push-it install` (note: a source build has no macOS glow; release binaries are built with `-tags glowhelper` - see [glow.md](glow.md)).

`push-it install` with no flags asks three yes/no questions (sound, Hue, glow). Or be explicit:

```sh
push-it install --sound --glow        # pick components
push-it install --all                 # everything
push-it install --sound --yes         # non-interactive
```

`--sound`, `--hue`, and `--glow` each enable that component and leave the others alone. On a brand-new machine (no config yet) they start from everything off, so `install --sound` gives you sound only. `--all` enables everything. After every install push-it prints `enabled: sound=... hue=... glow=...`.

Then drop `.mp3` or `.wav` files into the clips directory it prints (or cut your own - see [make-your-own-clips.md](make-your-own-clips.md)) and push something.

## What install changes

- Writes `config.json` (mode 0600) to your OS config dir: `~/.config/push-it/` on Linux, `~/Library/Application Support/push-it/` on macOS, `%APPDATA%\push-it\` on Windows. Override with `PUSH_IT_CONFIG_DIR`.
- Wires the git hook. If `git config --global core.hooksPath` is unset, it is pointed at `<config dir>/hooks/`. If you already have one, a marker-delimited block is appended to its `pre-push` (created if missing):

  ```sh
  # >>> push-it >>>
  '/path/to/push-it' hook pre-push "$@" || true
  # <<< push-it <<<
  ```

- Glow only: on Linux extracts and enables the GNOME Shell extension (log out and back in once - Wayland cannot hot-load extensions); on macOS extracts the helper app next to the config; on Windows nothing to install. See [glow.md](glow.md).

Everything it did is recorded in `config.json` under `install_state`, and `push-it uninstall` reverses exactly that - your own hooks are restored byte-for-byte.

## Kill switches

| Variable | Effect |
|---|---|
| `NO_PUSH_IT=1` | skip everything |
| `NO_SOUND=1` | skip the clip |
| `NO_RAINBOW=1` | skip Hue |
| `NO_GLOW=1` | skip the screen glow |

Example: `NO_PUSH_IT=1 git push` for a quiet push.

## Checking on it

`push-it doctor` prints the config path, component status, clip count, Hue reachability, and which glow backend this build has. Errors during a push never block the push; they go to `<config dir>/push-it.log`.

## Uninstall

```sh
push-it uninstall          # asks whether to delete config + clips (default: keep)
push-it uninstall --yes    # keep them, no questions
```

The binary itself is left for you to remove (`rm $(which push-it)` or your package manager).
