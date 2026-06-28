/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
export type SearchPoolProvider = 'tavily' | 'exa'

export type SearchPoolSummary = {
  provider: SearchPoolProvider
  total: number
  enabled: number
  disabled: number
  linked: number
  monthly_capacity: number
}

export type SearchPoolAccount = {
  id: number
  provider: SearchPoolProvider
  name: string
  key_fingerprint: string
  key_tail: string
  api_key_id?: string
  project_id?: string
  monthly_limit: number
  base_url?: string
  proxy?: string
  paid_until: number
  remark?: string
  enabled: boolean
  status: number
  channel_id: number
  key_index: number
  last_error?: string
  created_at: number
  updated_at: number
}

export type SearchPoolImportRequest = {
  text: string
  format?: 'auto' | 'csv' | 'json' | 'lines'
  default_provider: SearchPoolProvider
  replace?: boolean
  connect?: boolean
  generate_api_key?: boolean
  group?: string
  tag?: string
  channel_prefix?: string
  token_name?: string
  token_user_id?: number
  token_group?: string
  token_unlimited?: boolean
  token_quota?: number
}

export type SearchPoolGeneratedToken = {
  id: number
  name: string
  key: string
  full_key: string
  group: string
}

export type SearchPoolApplyResult = {
  provider: SearchPoolProvider
  channel_id: number
  channel_name: string
  key_count: number
  models: string
  base_url: string
  proxy_warnings?: string[]
}

export type SearchPoolApplyResponse = {
  success: boolean
  message?: string
  base_url?: string
  channels?: SearchPoolApplyResult[]
  token?: SearchPoolGeneratedToken
}

export type SearchPoolImportResult = {
  imported: number
  skipped: number
  errors: string[]
  apply?: SearchPoolApplyResponse
}

export type SearchPoolSyncResult = {
  provider: SearchPoolProvider
  channel_id?: number
  key_index?: number
  success: boolean
  message?: string
  upstream_status?: number
}

export type SearchPoolSyncResponse = {
  success: boolean
  message?: string
  data?: SearchPoolSyncResult[]
}
