# Migrating from a hand-rolled hook

If you previously had your own `pre-push` that played a clip (and maybe a separate Hue script), here is how to move over without losing your clips.

1. **Install push-it** - `push-it install --sound` (add `--hue`/`--glow` as you like). If your old hook lives in a global `core.hooksPath` directory, push-it appends itself to it; it does not remove anything.
2. **Copy your clips** into the directory `push-it doctor` prints under `clips:`. Any `.mp3`/`.wav` works; no manifest is needed - the filename is the label.
3. **Move Hue credentials** - run `push-it install --hue` and paste the bridge, key, and light ID, or export `PUSH_IT_HUE_BRIDGE` / `PUSH_IT_HUE_KEY` / `PUSH_IT_HUE_LIGHT` from your secret manager.
4. **Retire the old scripts** - delete the old player/Hue lines from your `pre-push` (keep the `# >>> push-it >>>` block) so the clip doesn't play twice.
5. **Test** - `push-it play`, `push-it hue`, then push to a scratch branch.

Kill-switch names are compatible with common conventions: `NO_PUSH_IT=1` silences everything, `NO_RAINBOW=1` skips the light.
