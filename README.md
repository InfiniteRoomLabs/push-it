# push-it

Celebrate every `git push`: a random "push it" clip, a rainbow burst on a Philips Hue light, and an animated rainbow frame around your screen for exactly as long as the clip plays. Each effect is optional. One binary, no runtime dependencies.

Ships no audio  -  you cut your own clips from a track you own with the bundled toolkit. See [docs/make-your-own-clips.md](docs/make-your-own-clips.md).

## Install

```sh
go install github.com/InfiniteRoomLabs/push-it/cmd/push-it@latest
push-it install            # asks: sound? hue? glow?
```

Prebuilt binaries for Linux, macOS, and Windows (amd64 + arm64) and a one-line `install.sh` come with the first tagged release. Details, kill switches, and uninstall: [docs/install.md](docs/install.md).

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
