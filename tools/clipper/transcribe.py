# /// script
# requires-python = ">=3.11"
# dependencies = [
#     "faster-whisper>=1.1,<2",
# ]
# ///
"""Transcribe a track with word-level timestamps.

Usage:
    uv run tools/clipper/transcribe.py SOURCE.mp3 -o transcript.json [--model small.en]

The output is a JSON list of {"word", "start", "end"} objects - the input
format for `push-it clips cut`. faster-whisper's `av` dependency bundles
ffmpeg's libraries, so no system ffmpeg is required.
"""

import argparse
import json
import sys
from pathlib import Path

from faster_whisper import WhisperModel


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("source", type=Path, help="audio file (mp3, wav, ...)")
    ap.add_argument("-o", "--output", type=Path, default=Path("transcript.json"))
    ap.add_argument("--model", default="small.en", help="faster-whisper model name (default: small.en)")
    args = ap.parse_args()

    # Word-level timestamps are the point: segment-level is too coarse to cut
    # two-second clips. VAD is off because music has no real silence.
    model = WhisperModel(args.model, device="cpu", compute_type="int8")
    print(f"transcribing {args.source} with {args.model} ...", file=sys.stderr)
    segments, _info = model.transcribe(str(args.source), word_timestamps=True, vad_filter=False)

    words = []
    for seg in segments:
        for w in seg.words or []:
            words.append({"word": w.word.strip().lower(), "start": round(w.start, 3), "end": round(w.end, 3)})
            print(f"  [{w.start:6.2f}-{w.end:6.2f}] {w.word!r}", file=sys.stderr)

    args.output.write_text(json.dumps(words, indent=2) + "\n")
    print(f"wrote {len(words)} words to {args.output}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
