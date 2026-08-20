# Make your own clips

push-it ships no audio. Clips are short excerpts you cut from a track you own a copy of, for your own use on your own machine. Distributing the resulting clips is your responsibility, not something this project does or encourages.

The pipeline is three steps: transcribe -> cut -> review.

## 0. Prerequisites

- `push-it` installed ([install.md](install.md))
- [`uv`](https://docs.astral.sh/uv/) for the one-time transcription step (the only Python in the project)
- Your source track as `.mp3` or `.wav`. Other formats: convert once with whatever you have (`ffmpeg -i in.m4a out.mp3`, Audacity, ...). push-it itself never needs ffmpeg.

## 1. Transcribe with word timestamps

```sh
uv run tools/clipper/transcribe.py source.mp3 -o transcript.json
```

This runs [faster-whisper](https://github.com/SYSTRAN/faster-whisper) (`small.en` on CPU; a few minutes for a 4-minute track) and writes a list of `{"word","start","end"}` entries. Use `--model medium.en` for better accuracy at the cost of time and RAM.

## 2. Cut candidates

```sh
push-it clips cut -o candidates/ source.mp3 transcript.json
```

Flags go before the file arguments.

Defaults find phrases that start with "push it" and may continue with "push", "it", "real", "good" - up to 4 s long, with at most 0.5 s between words - and write each as a padded WAV (`001-push-it.wav`, `002-push-it-push-it-real-good.wav`, ...) plus `candidates.json`.

Tune for your track:

| Flag | Default | Meaning |
|---|---|---|
| `--phrase "push it"` | `push it` | words a clip must start with, in order |
| `--allow real,good` | `real,good` | extra words allowed inside a clip |
| `--gap 0.5` | `0.5` | max silence inside a phrase (s) |
| `--max 4.0` | `4.0` | max clip length (s) |
| `--pad 0.3` | `0.3` | padding before/after (s) |

Example for a different hook line: `push-it clips cut --phrase "ship it" --allow "now,yeah" -o candidates/ track.mp3 transcript.json`.

## 3. Review and keep

```sh
push-it clips review candidates/
```

Each candidate plays; answer `k` (keep), `s` (skip), `r` (replay), or `q` (quit). Keepers are moved into your configured clips dir, or use `push-it clips review --keep-to DIR candidates/` to choose another (flags go before the file argument here too). Whisper timestamps are good, not perfect - expect to skip a few clipped or mistimed ones.

Quit with `q` any time; re-running `push-it clips review candidates/` skips the ones you already kept.

Done: `push-it play` plays a random keeper; `git push` does the rest.
