# Exa Relay Module

This fork adds an Exa search relay channel alongside the Tavily module. It uses
New API's normal token authentication, channel key pools, groups, quota billing,
consume logs, and admin channel management.


The aggregated Tavily/Exa key-pool manager is documented in `docs/search-pool.md`.

## Channel Setup

1. Add a channel with type `Exa` (`60`).
2. Keep the base URL empty or use:

```text
https://api.exa.ai
```

3. Put one Exa API key per line when using multi-key mode.
4. Set models:

```text
exa-search,exa-contents
```

5. Assign groups and create downstream New API tokens as usual.

Default model prices are per request:

```text
exa-search    0.008 USD/request
exa-contents  0.008 USD/request
```

You can override these in the existing model pricing settings.

## Relay Endpoints

Downstream clients call your New API base URL with a New API token.

Search:

```bash
curl https://your-domain.example.com/exa/search \
  -H "Authorization: Bearer <new-api-token>" \
  -H "Content-Type: application/json" \
  -d '{"query":"latest LLM research","type":"auto","contents":{"highlights":true}}'
```

Contents:

```bash
curl https://your-domain.example.com/exa/contents \
  -H "X-API-Key: <new-api-token>" \
  -H "Content-Type: application/json" \
  -d '{"urls":["https://example.com"],"text":true}'
```

The relay forwards to Exa with the selected upstream channel key:

```text
x-api-key: <exa-upstream-key>
```

## Billing and Key Pool Usage

- Billing unit: one Exa request per successful `/exa/search` or `/exa/contents`
  call.
- The local table `exa_key_usages` tracks each channel key's monthly request
  usage, limit, reset time, last sync time, last error, key fingerprint, and key
  tail.
- When a key reaches its local monthly limit, it is auto-disabled so the key pool
  can move to the next available key.

Click `Key Usage` in the default channel list to open the usage dialog. The same
screen supports refresh, local reset, per-key limit edits, and optional upstream
usage sync.

## Exa Usage Sync

Exa's authoritative per-key usage endpoint is under the Team Management API:

```text
GET https://admin-api.exa.ai/team-management/api-keys/{id}/usage
```

The service key is sent as `x-api-key`. To sync a key from the UI:

1. Open `Key Usage` for the Exa channel.
2. Fill the `API Key ID` field with the Exa API key ID (not the secret key).
3. Click `Sync` or `Sync All`.

Optional custom admin base URL can be set in channel `other_info` JSON:

```json
{
  "exa_admin_base_url": "https://admin-api.exa.ai/team-management"
}
```

The sync parser reads `cost_breakdown[].quantity` from Exa's official response
and stores the summed quantity as local used requests. If Exa later returns a
direct request counter, fallback fields such as `used_requests` and
`monthly_limit_requests` are also supported.

## Admin API

```bash
curl -H "Authorization: Bearer <admin-token>" \
  https://your-domain.example.com/api/channel/<channel-id>/exa/usage
```

Reset all local usage:

```bash
curl -X POST \
  -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  https://your-domain.example.com/api/channel/<channel-id>/exa/usage/reset \
  -d '{}'
```

Sync one key:

```bash
curl -X POST \
  -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  https://your-domain.example.com/api/channel/<channel-id>/exa/usage/sync \
  -d '{"key_index":0}'
```
