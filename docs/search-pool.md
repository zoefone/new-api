# Search Pool Manager (Tavily + Exa)

This fork adds an admin Search Pool module that imports upstream Tavily/Exa
accounts or API keys, stores them as a provider key pool, and applies them into
New API relay channels with the same downstream authentication, grouping,
billing, logs, and model-limit flow as normal New API channels.

## What it does

- Imports Tavily and Exa upstream keys in lines, CSV, or JSON format.
- Hides the upstream secret after import; the UI shows key tail and fingerprint.
- Creates/updates provider relay channels automatically:
  - Tavily channel type `59`: `tavily-search,tavily-extract`
  - Exa channel type `60`: `exa-search,exa-contents`
- Enables multi-key polling on the generated channels.
- Links each imported account to its channel/key index for auditing.
- Creates per-key usage rows and monthly request/credit limits.
- Optionally generates a downstream New API token with model limits for all
  Tavily/Exa relay models.
- Supports per-account base URL and proxy; the relay uses those overrides when
  the linked key is selected.

## Admin UI

Open:

```text
/search-pool
```

or use the sidebar item `Admin -> Search Pool`.

One-click import fields:

- Provider: default provider for plain one-key-per-line imports.
- Group: New API group assigned to generated channels and optional token.
- Tag: channel tag used to find/update generated pool channels. Defaults to
  `search-pool`.
- Replace duplicates: update existing records with the same provider and key
  fingerprint.
- Connect after import: immediately create/update New API channels.
- Generate API key: creates a downstream New API token and shows the full key
  once. Copy it immediately.

## Import formats

Plain lines:

```text
tvly-key-1
tvly-key-2
```

CSV with header:

```csv
provider,api_key,api_key_id,monthly_limit,base_url,proxy,remark
tavily,tvly-key-1,project-a,1000,https://api.tavily.com,,main tavily key
exa,exa-key-1,exa-api-key-id,1000,https://api.exa.ai,http://127.0.0.1:7890,team key
```

JSON:

```json
{
  "accounts": [
    {
      "provider": "tavily",
      "api_key": "tvly-key-1",
      "project_id": "project-a",
      "monthly_limit": 1000
    },
    {
      "provider": "exa",
      "api_key": "exa-key-1",
      "api_key_id": "exa-api-key-id",
      "monthly_limit": 1000
    }
  ]
}
```

## Admin API

List summaries:

```bash
curl -H "Authorization: Bearer <admin-newapi-token>" \
  https://your-domain.example.com/api/search_pool/summary
```

Import and apply:

```bash
curl -X POST https://your-domain.example.com/api/search_pool/accounts/import \
  -H "Authorization: Bearer <admin-newapi-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "default_provider":"tavily",
    "text":"tvly-key-1\ntvly-key-2",
    "connect":true,
    "generate_api_key":true,
    "group":"default",
    "token_name":"Search Pool API Key",
    "token_unlimited":true
  }'
```

Apply existing pool records into New API channels:

```bash
curl -X POST https://your-domain.example.com/api/search_pool/apply \
  -H "Authorization: Bearer <admin-newapi-token>" \
  -H "Content-Type: application/json" \
  -d '{"provider":"all","group":"default","tag":"search-pool"}'
```

Sync upstream usage where provider sync is configured:

```bash
curl -X POST https://your-domain.example.com/api/search_pool/sync \
  -H "Authorization: Bearer <admin-newapi-token>" \
  -H "Content-Type: application/json" \
  -d '{"provider":"all","tag":"search-pool"}'
```

## Downstream usage

After channels are applied and a downstream token is generated, clients call the
relay base URL, not Tavily/Exa directly:

```bash
curl https://your-domain.example.com/tavily/search \
  -H "Authorization: Bearer <new-api-token>" \
  -H "Content-Type: application/json" \
  -d '{"query":"latest LLM news"}'

curl https://your-domain.example.com/exa/search \
  -H "Authorization: Bearer <new-api-token>" \
  -H "Content-Type: application/json" \
  -d '{"query":"latest LLM news"}'
```

