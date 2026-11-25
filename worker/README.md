# Badge worker

The script name is `stageproof-index-badges`. The workers.dev subdomain is `stageproof`, so the worker is `https://stageproof-index-badges.stageproof.workers.dev`.

`workers_dev` stays on. That hostname is the badge address. `preview_urls` is off, so a deploy does not publish a version URL.

A push to `main` does not deploy this worker. Deploy by running the `Deploy worker` workflow. It is `workflow_dispatch` only. From `worker/`, with `CLOUDFLARE_API_TOKEN` and `CLOUDFLARE_ACCOUNT_ID` set, it runs:

```bash
npx --yes wrangler@4.168.0 deploy
```

The token is limited to Workers Scripts edit. Do not use an account-wide API key. Add both values as Actions secrets on `stageproof/index` before the first run. The workflow stops when either secret is missing, and it does not print the token.

## Check

After a deploy, these six requests are the check. The indexed pair is Traefik 42.4.3 to 43.0.0.

```bash
base=https://stageproof-index-badges.stageproof.workers.dev

# Indexed pair. HTTP 200, image/svg+xml, and the body is not the not-indexed image.
curl -sS -D - -o /tmp/badge.svg -G "$base/badge.svg" \
  --data-urlencode chart=traefik \
  --data-urlencode from=42.4.3 \
  --data-urlencode registry=https://traefik.github.io/charts \
  --data-urlencode to=43.0.0

# The same pair with a trailing slash on the registry. Same image.
curl -sS -D - -o /tmp/badge-slash.svg -G "$base/badge.svg" \
  --data-urlencode chart=traefik \
  --data-urlencode from=42.4.3 \
  --data-urlencode registry=https://traefik.github.io/charts/ \
  --data-urlencode to=43.0.0

# Unknown chart. HTTP 200 and the not-indexed image.
# A known chart without that pair is HTTP 200 and says "pair not indexed".
# The live script says that only after this workflow has deployed the current source.
curl -sS -D - -o /tmp/badge-unknown.svg -G "$base/badge.svg" \
  --data-urlencode chart=not-a-real-chart \
  --data-urlencode from=1.0.0 \
  --data-urlencode registry=https://charts.example/missing \
  --data-urlencode to=1.1.0

# Empty query. HTTP 200 and the not-indexed image.
curl -sS -D - -o /tmp/badge-empty.svg "$base/badge.svg"

# POST. HTTP 405.
curl -sS -D - -o /dev/null -X POST "$base/badge.svg"

# Any other path. HTTP 404.
curl -sS -D - -o /dev/null "$base/"
```
