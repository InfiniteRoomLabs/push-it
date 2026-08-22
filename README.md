# push-it

Celebrate every `git push`: a random "push it" clip, a rainbow burst on a Philips Hue light, and an animated rainbow glow around your screen for exactly as long as the clip plays. Each effect is optional. One binary, no runtime dependencies beyond your desktop's audio server (PulseAudio/PipeWire on Linux).

Ships no audio  -  you cut your own clips from a track you own with the bundled toolkit. See [docs/make-your-own-clips.md](docs/make-your-own-clips.md).

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/InfiniteRoomLabs/push-it/main/install.sh | sh -s -- --all
```

That downloads the latest release for your OS/arch (Linux and macOS, amd64 and arm64), verifies its checksum, puts `push-it` in `~/.local/bin`, and runs `push-it install --all`. Drop `--all` to be asked per component, or pass `--sound`, `--hue`, `--glow` explicitly. Windows: grab the zip from the [releases page](https://github.com/InfiniteRoomLabs/push-it/releases) and run `push-it install`. From source: `go install github.com/InfiniteRoomLabs/push-it/cmd/push-it@latest` (note: a source build has no macOS glow; see [docs/glow.md](docs/glow.md)).

Details, kill switches, and uninstall: [docs/install.md](docs/install.md).

## Usage

```sh
git push                   # that's it
push-it play               # one random clip, on demand
push-it doctor             # what's configured, what's reachable
NO_PUSH_IT=1 git push      # quiet push
```

## Docs

- [docs/install.md](docs/install.md)  -  install, uninstall, `doctor`
- [docs/make-your-own-clips.md](docs/make-your-own-clips.md)  -  transcribe, cut, review
- [docs/hue.md](docs/hue.md)  -  Philips Hue setup
- [docs/glow.md](docs/glow.md)  -  screen glow per platform
- [docs/migrating.md](docs/migrating.md)  -  moving from a hand-rolled hook

## License

MIT  -  see [LICENSE](LICENSE).
