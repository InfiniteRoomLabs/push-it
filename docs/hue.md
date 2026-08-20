# Philips Hue

On push, push-it saves one light's state, runs it through the hue wheel for about 3.5 s, and restores it.

## Setup

You need the bridge address, an API key, and a light ID.

1. Find the bridge: your router's client list, the Hue app (Settings -> My Hue System -> bridge -> i), or `https://discovery.meethue.com/`.
2. Create an API key: press the bridge's link button, then within 30 s:

   ```sh
   curl -sk -X POST https://<bridge>/api -d '{"devicetype":"push-it#laptop"}'
   ```

   The response contains `"username": "<key>"`.
3. List lights to find the ID: `curl -sk https://<bridge>/api/<key>/lights | jq 'map_values(.name)'`.
4. `push-it install --hue` and answer the prompts, or set `PUSH_IT_HUE_BRIDGE`, `PUSH_IT_HUE_KEY`, `PUSH_IT_HUE_LIGHT` in the environment - env values are applied on top of the config file and used as the prompt defaults, so the key can live in your secret manager instead of on disk. With `--yes`, env values configure Hue without prompting (Hue is skipped if bridge or key is still empty). Either way, run `push-it install --hue` once so the bridge certificate gets pinned.

`push-it hue` fires the burst on demand; `push-it doctor` checks reachability.

## Notes

- Hue bridges present TLS certificates no public CA signs. Instead of skipping verification, push-it pins the bridge's certificate on first contact (`push-it install --hue` prints the fingerprint and stores it in `config.json` under `hue.cert_sha256`) and refuses to talk to anything else afterwards. If you replace the bridge, re-run `push-it install --hue`; it shows the old and new fingerprints and asks before trusting the new one, then re-pins to the new certificate.
- To clear a stored pin (e.g. you're resetting the bridge relationship), re-run `push-it install --hue`; the prompt walks you through trusting whatever certificate the bridge presents now.
- Every call has a 2 s timeout; if the bridge is unreachable the push is unaffected and the error goes to the log.
- Overlapping pushes do not stack bursts; a lock file skips the second one so the save/restore can't fight.
- `NO_RAINBOW=1 git push` skips Hue for one push.
