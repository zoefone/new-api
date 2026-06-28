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
import { api } from '@/lib/api'
import type {
  SearchPoolAccount,
  SearchPoolApplyResponse,
  SearchPoolImportRequest,
  SearchPoolImportResult,
  SearchPoolProvider,
  SearchPoolSummary,
  SearchPoolSyncResponse,
} from './types'

type ApiEnvelope<T> = {
  success: boolean
  message?: string
  data: T
}

const actionConfig = {
  skipBusinessError: true,
  skipErrorHandler: true,
}

export async function getSearchPoolSummary(): Promise<SearchPoolSummary[]> {
  const res = await api.get<ApiEnvelope<{ providers: SearchPoolSummary[] }>>(
    '/api/search_pool/summary'
  )
  return res.data.data.providers
}

export async function getSearchPoolAccounts(params?: {
  provider?: SearchPoolProvider | 'all'
  enabled?: boolean
}): Promise<SearchPoolAccount[]> {
  const res = await api.get<ApiEnvelope<SearchPoolAccount[]>>(
    '/api/search_pool/accounts',
    {
      params: {
        provider: params?.provider === 'all' ? undefined : params?.provider,
        enabled: params?.enabled,
      },
    }
  )
  return res.data.data
}

export async function importSearchPoolAccounts(
  data: SearchPoolImportRequest
): Promise<SearchPoolImportResult> {
  const res = await api.post<ApiEnvelope<SearchPoolImportResult>>(
    '/api/search_pool/accounts/import',
    data,
    actionConfig
  )
  if (!res.data.success) throw new Error(res.data.message || 'Import failed')
  return res.data.data
}

export async function updateSearchPoolAccount(
  id: number,
  data: Partial<SearchPoolAccount>
): Promise<SearchPoolAccount> {
  const res = await api.put<ApiEnvelope<SearchPoolAccount>>(
    `/api/search_pool/accounts/${id}`,
    data,
    actionConfig
  )
  if (!res.data.success) throw new Error(res.data.message || 'Update failed')
  return res.data.data
}

export async function deleteSearchPoolAccount(id: number): Promise<void> {
  const res = await api.delete<ApiEnvelope<null>>(
    `/api/search_pool/accounts/${id}`,
    actionConfig
  )
  if (!res.data.success) throw new Error(res.data.message || 'Delete failed')
}

export async function applySearchPool(data: {
  provider?: SearchPoolProvider | 'all'
  group?: string
  tag?: string
  channel_prefix?: string
  create_token?: boolean
  token_name?: string
  token_user_id?: number
  token_group?: string
  token_unlimited?: boolean
  token_quota?: number
}): Promise<SearchPoolApplyResponse> {
  const res = await api.post<SearchPoolApplyResponse>(
    '/api/search_pool/apply',
    data,
    actionConfig
  )
  if (!res.data.success) throw new Error(res.data.message || 'Apply failed')
  return res.data
}

export async function syncSearchPoolUsage(data: {
  provider?: SearchPoolProvider | 'all'
  tag?: string
}): Promise<SearchPoolSyncResponse> {
  const res = await api.post<SearchPoolSyncResponse>(
    '/api/search_pool/sync',
    data,
    actionConfig
  )
  return res.data
}
