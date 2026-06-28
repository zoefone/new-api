# Tavily Relay Module

This fork adds a Tavily search relay channel to New API. It reuses New API user
tokens, channel key pools, quota billing, logs, groups, and admin management.

The same fork also includes an Exa relay module. See `docs/exa-relay.md` for
the Exa channel type, `/exa/search`, `/exa/contents`, and Exa key-pool usage
sync details.

## Do I need to upload this to GitHub?

Not strictly. The code already exists in this local repository:

```bash
/root/new-api
```

You should push it to your own GitHub repository if you want any of these:

- deploy from a remote server with `git clone`
- build Docker images in GitHub Actions or another CI runner
- keep a long-lived fork and merge upstream New API updates later
- share the source with users or collaborators

Recommended long-term workflow:

```bash
cd /root/new-api
git remote -v
git remote rename origin upstream
git remote add origin git@github.com:<your-org>/<your-newapi-fork>.git
git checkout -b feature/tavily-relay
git add .
git commit -m "add tavily relay channel"
git push -u origin feature/tavily-relay
```

Then open a PR into your own main branch. Keep `upstream` pointed at
`QuantumNous/new-api` so future updates can be merged with fewer conflicts.

## Main Changed Files

Backend:

- `relay/tavily/handler.go`: Tavily `/search` and `/extract` relay, billing, key pool selection.
- `relay/tavily/handler_test.go`: credit calculation tests.
- `relay/channel/tavily/adaptor.go`: lightweight channel adaptor for model listing and channel registration.
- `model/tavily_usage.go`: local per-key monthly credit usage table.
- `controller/tavily.go`: admin usage and local reset APIs.
- `middleware/tavily.go`: `X-API-Key` compatibility for New API tokens.
- `router/relay-router.go`: public `/tavily/search` and `/tavily/extract` routes.
- `router/api-router.go`: admin Tavily usage routes.
- `constant/*`, `common/*`, `relay/constant/*`: channel/API/endpoint/relay-mode constants.
- `setting/ratio_setting/model_ratio.go`: default Tavily model prices.

Frontend:

- `web/default/src/features/channels/components/dialogs/tavily-usage-dialog.tsx`: Tavily key usage dialog.
- `web/default/src/features/channels/components/channels-columns.tsx`: Tavily usage entry in channel list.
- `web/default/src/features/channels/api.ts`: Tavily usage/reset/sync/settings API client.
- `web/default/src/features/channels/constants.ts`: channel type 59.
- `web/default/src/features/channels/lib/channel-type-config.ts`: Tavily channel form config.
- `web/classic/src/constants/channel.constants.js`: classic UI channel type.
- `web/classic/src/components/table/channels/modals/EditChannelModal.jsx`: key prompt.
- `web/classic/src/helpers/render.jsx`: channel icon.

## Deployment

### Recommended: build image outside the 2C2G server

The Dockerfile builds both web frontends and the Go binary. Frontend build can
consume enough memory to freeze a small 2C2G VPS. Prefer GitHub Actions, another
CI runner, or a larger temporary build machine.

```bash
cd /root/new-api
docker build -t <your-registry>/new-api:tavily .
docker push <your-registry>/new-api:tavily
```

On the 2C2G server, only pull and run the image:

```bash
docker pull <your-registry>/new-api:tavily
```

Edit `docker-compose.yml`:

```yaml
services:
  new-api:
    image: <your-registry>/new-api:tavily
```

Then deploy:

```bash
docker compose up -d
```

### Local source build

Only use this on a machine with enough memory:

```bash
cd /root/new-api/web
bun install --frozen-lockfile
cd default && bun run build
cd ../classic && bun run build

cd /root/new-api
go build -ldflags "-s -w" -o new-api
./new-api --log-dir ./logs
```

The Go root package embeds `web/default/dist` and `web/classic/dist`, so both
frontend builds must exist before `go build` or `go test ./...` can compile the
root package.


The aggregated Tavily/Exa key-pool manager is documented in `docs/search-pool.md`.

## Admin Setup

1. Log in as admin.
2. Open channel management.
3. Add a channel with type `Tavily`.
4. Base URL can stay as:

```text
https://api.tavily.com
```

5. Add Tavily upstream API keys in the channel key field.
   In multi-key mode, put one Tavily key per line.
6. Enable multi-key mode if you have a key pool.
   Polling mode is usually easier to audit than random mode.
7. Set supported models:

```text
tavily-search,tavily-extract
```

8. Assign groups as usual.
9. Create or reuse a New API token for your downstream users.

Default model price is `0.008` USD per Tavily credit for both:

```text
tavily-search
tavily-extract
```

You can override this in the existing model pricing settings.

## Client Usage

Use the New API token, not the upstream Tavily key.

`Authorization: Bearer`:

```bash
curl https://your-domain.example.com/tavily/search \
  -H "Authorization: Bearer <new-api-token>" \
  -H "Content-Type: application/json" \
  -d '{"query":"latest OpenAI news","search_depth":"basic"}'
```

`X-API-Key` compatibility:

```bash
curl https://your-domain.example.com/tavily/extract \
  -H "X-API-Key: <new-api-token>" \
  -H "Content-Type: application/json" \
  -d '{"urls":["https://example.com"],"extract_depth":"basic"}'
```

The upstream request is sent to Tavily with the selected channel key:

```text
Authorization: Bearer <tavily-upstream-key>
```

## Billing Rules

Search:

- `search_depth=advanced`: 2 Tavily credits
- `basic`, `fast`, `ultra-fast`, missing, or unknown: 1 Tavily credit

Extract:

- Basic: every 5 successful URL extractions costs 1 Tavily credit.
- Advanced: every 5 successful URL extractions costs 2 Tavily credits.
- Failed URL extractions are not charged.
- Request pre-consumption is estimated from requested URL count, then settled
  after the upstream response based on `len(results)`.

## Feature Checklist

Implemented:

- Tavily channel type `59`.
- Public relay endpoints:
  - `/tavily/search`
  - `/tavily/extract`
- Downstream auth through New API user tokens:
  - `Authorization: Bearer <new-api-token>`
  - `X-API-Key: <new-api-token>`
- Upstream auth through the selected Tavily channel key.
- New API group, token, channel, key-pool, log, quota, and pricing flows.
- Request-based Tavily credit billing:
  - search depth based credits
  - extract success-result based settlement
- Local per-key monthly usage tracking.
- Per-key local reset.
- Manual official Tavily `/usage` synchronization.
- Editable per-key local monthly limit.
- Editable per-key project label.
- Default UI channel list entry and Tavily usage dialog.
- Classic UI can create/edit Tavily channels.
- GitHub Actions image build workflow for GHCR.

Still intentionally left small or manual:

- No scheduled background `/usage` sync yet.
- Classic UI does not expose the Tavily usage dialog.
- The usage parser supports Tavily's documented `key.usage` and `key.limit`
  fields plus several common fallback names; unsupported future response shapes
  may need a parser update.

## Key Pool Usage

The local table `tavily_key_usages` tracks per-channel, per-key usage:

- key index
- key fingerprint and tail
- project label
- monthly limit, default `1000`
- used credits
- next local reset time
- last official `/usage` sync time
- last error

Local usage is shown in the default frontend channel list by clicking `Key Usage`
on a Tavily channel. The dialog can refresh local state, synchronize official
Tavily usage, save a project label/monthly local limit, or reset local counters.

Admin APIs:

```bash
curl -H "Authorization: Bearer <admin-token>" \
  https://your-domain.example.com/api/channel/<channel-id>/tavily/usage
```

Reset all local usage for a Tavily channel:

```bash
curl -X POST \
  -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  https://your-domain.example.com/api/channel/<channel-id>/tavily/usage/reset \
  -d '{}'
```

Reset one key:

```bash
curl -X POST \
  -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  https://your-domain.example.com/api/channel/<channel-id>/tavily/usage/reset \
  -d '{"key_index":0}'
```

Resetting here only resets New API's local counter. It does not reset Tavily's
official account quota.

Synchronize all keys with Tavily's official `/usage` endpoint:

```bash
curl -X POST \
  -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  https://your-domain.example.com/api/channel/<channel-id>/tavily/usage/sync \
  -d '{}'
```

Synchronize one key:

```bash
curl -X POST \
  -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  https://your-domain.example.com/api/channel/<channel-id>/tavily/usage/sync \
  -d '{"key_index":0}'
```

The sync call sends:

```text
GET <channel-base-url>/usage
Authorization: Bearer <tavily-upstream-key>
```

The documented Tavily response shape is:

```json
{
  "key": {
    "usage": 150,
    "limit": 1000,
    "search_usage": 100,
    "extract_usage": 25,
    "crawl_usage": 15,
    "map_usage": 7,
    "research_usage": 3
  },
  "account": {
    "current_plan": "Bootstrap",
    "plan_usage": 500,
    "plan_limit": 15000,
    "paygo_usage": 25,
    "paygo_limit": 100
  }
}
```

New API stores `key.usage` as `used_credits` and `key.limit` as
`monthly_limit_credits`.

Update one key's local settings:

```bash
curl -X POST \
  -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  https://your-domain.example.com/api/channel/<channel-id>/tavily/usage/update \
  -d '{"key_index":0,"monthly_limit_credits":1000,"project_id":"project-a"}'
```

## Current Limitations

- Official Tavily `/usage` synchronization is manual. There is no scheduled
  background sync worker yet.
- Classic UI can create/edit Tavily channels, but the Tavily usage dialog is
  currently implemented only in the default UI.
- On small 2C2G servers, do not run full frontend builds during production
  traffic.

## Verification Commands

Low-load backend checks used during development:

```bash
cd /root/new-api
GOMAXPROCS=1 go test -p 1 -mod=readonly ./relay/tavily ./relay/channel/tavily ./controller ./model ./middleware ./router
GOMAXPROCS=1 go test -p 1 -mod=readonly ./relay/exa ./relay/channel/exa ./controller ./model ./middleware ./router
git diff --check
```

Full check after frontend dist exists:

```bash
cd /root/new-api
GOMAXPROCS=1 go test -p 1 -mod=readonly ./...
```
