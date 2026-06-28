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
import { useMemo, useState, type ReactNode } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  CheckCircle2,
  Copy,
  KeyRound,
  Link2,
  RefreshCcw,
  Trash2,
} from 'lucide-react'
import { toast } from 'sonner'
import { useTranslation } from 'react-i18next'
import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Textarea } from '@/components/ui/textarea'
import {
  applySearchPool,
  deleteSearchPoolAccount,
  getSearchPoolAccounts,
  getSearchPoolSummary,
  importSearchPoolAccounts,
  syncSearchPoolUsage,
  updateSearchPoolAccount,
} from './api'
import type {
  SearchPoolAccount,
  SearchPoolApplyResponse,
  SearchPoolProvider,
} from './types'

const providers: SearchPoolProvider[] = ['tavily', 'exa']

function providerLabel(provider: SearchPoolProvider | 'all') {
  if (provider === 'tavily') return 'Tavily'
  if (provider === 'exa') return 'Exa'
  return 'All'
}

function formatTime(value?: number) {
  if (!value) return '-'
  return new Date(value * 1000).toLocaleString()
}

function applyMessage(result?: SearchPoolApplyResponse) {
  if (!result) return ''
  const channelText = result.channels?.length
    ? result.channels
        .map((item) => `${providerLabel(item.provider)} #${item.channel_id}`)
        .join(', ')
    : 'no channel changed'
  const tokenText = result.token?.full_key ? '，API Key generated' : ''
  return `${channelText}${tokenText}`
}

export function SearchPool() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [provider, setProvider] = useState<SearchPoolProvider>('tavily')
  const [filterProvider, setFilterProvider] = useState<SearchPoolProvider | 'all'>(
    'all'
  )
  const [group, setGroup] = useState('default')
  const [tag, setTag] = useState('search-pool')
  const [tokenName, setTokenName] = useState('Search Pool API Key')
  const [importText, setImportText] = useState('')
  const [replace, setReplace] = useState(false)
  const [connectAfterImport, setConnectAfterImport] = useState(true)
  const [generateKeyAfterImport, setGenerateKeyAfterImport] = useState(false)
  const [lastToken, setLastToken] = useState('')

  const summaryQuery = useQuery({
    queryKey: ['search-pool', 'summary'],
    queryFn: getSearchPoolSummary,
  })

  const accountsQuery = useQuery({
    queryKey: ['search-pool', 'accounts', filterProvider],
    queryFn: () => getSearchPoolAccounts({ provider: filterProvider }),
  })

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ['search-pool'] })
  }

  const importMutation = useMutation({
    mutationFn: importSearchPoolAccounts,
    onSuccess: (result) => {
      invalidate()
      const message = `${t('Imported')}: ${result.imported}, ${t('Skipped')}: ${result.skipped}`
      if (result.errors?.length) toast.warning(result.errors.join('\n'))
      else toast.success(message)
      if (result.apply?.token?.full_key) setLastToken(result.apply.token.full_key)
      const applied = applyMessage(result.apply)
      if (applied) toast.success(applied)
    },
    onError: (error) => toast.error(error.message),
  })

  const applyMutation = useMutation({
    mutationFn: applySearchPool,
    onSuccess: (result) => {
      invalidate()
      if (result.token?.full_key) setLastToken(result.token.full_key)
      toast.success(applyMessage(result) || t('Applied'))
    },
    onError: (error) => toast.error(error.message),
  })

  const syncMutation = useMutation({
    mutationFn: syncSearchPoolUsage,
    onSuccess: (result) => {
      invalidate()
      const failed = result.data?.filter((item) => !item.success) ?? []
      if (failed.length) toast.warning(`${failed.length} ${t('sync tasks failed')}`)
      else toast.success(t('Usage synced'))
    },
    onError: (error) => toast.error(error.message),
  })

  const updateMutation = useMutation({
    mutationFn: ({ id, data }: { id: number; data: Partial<SearchPoolAccount> }) =>
      updateSearchPoolAccount(id, data),
    onSuccess: invalidate,
    onError: (error) => toast.error(error.message),
  })

  const deleteMutation = useMutation({
    mutationFn: deleteSearchPoolAccount,
    onSuccess: () => {
      invalidate()
      toast.success(t('Deleted'))
    },
    onError: (error) => toast.error(error.message),
  })

  const accounts = accountsQuery.data ?? []
  const summaryByProvider = useMemo(
    () => new Map((summaryQuery.data ?? []).map((item) => [item.provider, item])),
    [summaryQuery.data]
  )

  const handleImport = () => {
    if (!importText.trim()) {
      toast.error(t('Please enter API keys'))
      return
    }
    importMutation.mutate({
      text: importText,
      format: 'auto',
      default_provider: provider,
      replace,
      connect: connectAfterImport,
      generate_api_key: generateKeyAfterImport,
      group,
      tag,
      token_name: tokenName,
      token_group: group,
      token_unlimited: true,
    })
  }

  const handleCopyToken = async () => {
    if (!lastToken) return
    await navigator.clipboard.writeText(lastToken)
    toast.success(t('Copied'))
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Search Pool')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button
          variant='outline'
          onClick={() =>
            syncMutation.mutate({ provider: filterProvider, tag })
          }
          disabled={syncMutation.isPending}
        >
          <RefreshCcw />
          {t('Sync Usage')}
        </Button>
        <Button
          onClick={() =>
            applyMutation.mutate({ provider: filterProvider, group, tag })
          }
          disabled={applyMutation.isPending}
        >
          <Link2 />
          {t('Connect to NewAPI')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='flex flex-col gap-4'>
          <div className='grid gap-3 md:grid-cols-2 xl:grid-cols-4'>
            {providers.map((item) => {
              const summary = summaryByProvider.get(item)
              return (
                <Card key={item} size='sm'>
                  <CardHeader>
                    <CardTitle className='flex items-center gap-2'>
                      {providerLabel(item)}
                      <Badge variant='outline'>{summary?.enabled ?? 0} enabled</Badge>
                    </CardTitle>
                    <CardDescription>
                      {t('Aggregated provider key capacity')}
                    </CardDescription>
                  </CardHeader>
                  <CardContent>
                    <div className='grid grid-cols-2 gap-3 text-sm'>
                      <Metric label={t('Total')} value={summary?.total ?? 0} />
                      <Metric label={t('Linked')} value={summary?.linked ?? 0} />
                      <Metric
                        label={t('Disabled')}
                        value={summary?.disabled ?? 0}
                      />
                      <Metric
                        label={t('Monthly')}
                        value={summary?.monthly_capacity ?? 0}
                      />
                    </div>
                  </CardContent>
                </Card>
              )
            })}
            <Card size='sm' className='md:col-span-2'>
              <CardHeader>
                <CardTitle className='flex items-center gap-2'>
                  <KeyRound />
                  {t('Downstream API Key')}
                </CardTitle>
                <CardDescription>
                  {t('Generated NewAPI key for Tavily and Exa relay endpoints')}
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className='flex gap-2'>
                  <Input
                    readOnly
                    value={lastToken}
                    placeholder={t('Generate or import with API key enabled')}
                  />
                  <Button
                    variant='outline'
                    size='icon'
                    disabled={!lastToken}
                    onClick={handleCopyToken}
                  >
                    <Copy />
                  </Button>
                </div>
              </CardContent>
            </Card>
          </div>

          <div className='grid gap-4 xl:grid-cols-[minmax(360px,440px)_1fr]'>
            <Card>
              <CardHeader>
                <CardTitle>{t('One-click Import')}</CardTitle>
                <CardDescription>
                  {t(
                    'Paste one key per line, or CSV/JSON with provider, api_key, api_key_id, monthly_limit, base_url, proxy.'
                  )}
                </CardDescription>
              </CardHeader>
              <CardContent className='space-y-4'>
                <div className='grid gap-3 sm:grid-cols-2'>
                  <Field label={t('Provider')}>
                    <Select
                      value={provider}
                      onValueChange={(value) =>
                        setProvider(value as SearchPoolProvider)
                      }
                    >
                      <SelectTrigger className='w-full'>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent alignItemWithTrigger={false}>
                        <SelectGroup>
                          {providers.map((item) => (
                            <SelectItem key={item} value={item}>
                              {providerLabel(item)}
                            </SelectItem>
                          ))}
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                  </Field>
                  <Field label={t('Group')}>
                    <Input value={group} onChange={(e) => setGroup(e.target.value)} />
                  </Field>
                  <Field label='Tag'>
                    <Input value={tag} onChange={(e) => setTag(e.target.value)} />
                  </Field>
                  <Field label={t('Token Name')}>
                    <Input
                      value={tokenName}
                      onChange={(e) => setTokenName(e.target.value)}
                    />
                  </Field>
                </div>
                <Textarea
                  value={importText}
                  onChange={(e) => setImportText(e.target.value)}
                  className='min-h-48 font-mono text-xs'
                  placeholder={'tvly-key-1\ntvly-key-2\n\n# or CSV:\nprovider,api_key,api_key_id,monthly_limit\nexa,exa-key,api-key-id,1000'}
                />
                <div className='grid gap-2 text-sm sm:grid-cols-3'>
                  <ToggleLine
                    label={t('Replace duplicates')}
                    checked={replace}
                    onCheckedChange={setReplace}
                  />
                  <ToggleLine
                    label={t('Connect after import')}
                    checked={connectAfterImport}
                    onCheckedChange={setConnectAfterImport}
                  />
                  <ToggleLine
                    label={t('Generate API key')}
                    checked={generateKeyAfterImport}
                    onCheckedChange={setGenerateKeyAfterImport}
                  />
                </div>
                <Button
                  className='w-full'
                  onClick={handleImport}
                  disabled={importMutation.isPending}
                >
                  <CheckCircle2 />
                  {t('Import and Apply')}
                </Button>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className='flex items-center justify-between gap-2'>
                  <span>{t('Provider Accounts')}</span>
                  <Select
                    value={filterProvider}
                    onValueChange={(value) =>
                      setFilterProvider(value as SearchPoolProvider | 'all')
                    }
                  >
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent alignItemWithTrigger={false}>
                      <SelectGroup>
                        <SelectItem value='all'>{t('All')}</SelectItem>
                        {providers.map((item) => (
                          <SelectItem key={item} value={item}>
                            {providerLabel(item)}
                          </SelectItem>
                        ))}
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                </CardTitle>
                <CardDescription>
                  {t('Imported upstream keys are hidden; key tail and fingerprint are shown for audit.')}
                </CardDescription>
              </CardHeader>
              <CardContent>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t('Provider')}</TableHead>
                      <TableHead>{t('Name')}</TableHead>
                      <TableHead>{t('Key')}</TableHead>
                      <TableHead>{t('Limit')}</TableHead>
                      <TableHead>{t('Channel')}</TableHead>
                      <TableHead>{t('Status')}</TableHead>
                      <TableHead>{t('Updated')}</TableHead>
                      <TableHead className='text-right'>{t('Actions')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {accounts.map((account) => (
                      <TableRow key={account.id}>
                        <TableCell>{providerLabel(account.provider)}</TableCell>
                        <TableCell>
                          <div className='max-w-44 truncate font-medium'>
                            {account.name}
                          </div>
                          {account.last_error ? (
                            <div className='text-destructive max-w-56 truncate text-xs'>
                              {account.last_error}
                            </div>
                          ) : null}
                        </TableCell>
                        <TableCell>
                          <code className='bg-muted rounded px-1 py-0.5 text-xs'>
                            ...{account.key_tail}
                          </code>
                        </TableCell>
                        <TableCell>{account.monthly_limit}</TableCell>
                        <TableCell>
                          {account.channel_id > 0 ? (
                            <Badge variant='outline'>
                              #{account.channel_id}:{account.key_index}
                            </Badge>
                          ) : (
                            <Badge variant='secondary'>{t('Unlinked')}</Badge>
                          )}
                        </TableCell>
                        <TableCell>
                          <Switch
                            size='sm'
                            checked={account.enabled}
                            onCheckedChange={(checked) =>
                              updateMutation.mutate({
                                id: account.id,
                                data: { enabled: checked },
                              })
                            }
                          />
                        </TableCell>
                        <TableCell>{formatTime(account.updated_at)}</TableCell>
                        <TableCell className='text-right'>
                          <Button
                            variant='destructive'
                            size='icon-sm'
                            disabled={deleteMutation.isPending}
                            onClick={() => deleteMutation.mutate(account.id)}
                          >
                            <Trash2 />
                          </Button>
                        </TableCell>
                      </TableRow>
                    ))}
                    {!accounts.length ? (
                      <TableRow>
                        <TableCell colSpan={8} className='text-muted-foreground h-24 text-center'>
                          {accountsQuery.isLoading
                            ? t('Loading...')
                            : t('No accounts imported')}
                        </TableCell>
                      </TableRow>
                    ) : null}
                  </TableBody>
                </Table>
              </CardContent>
            </Card>
          </div>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

function Metric(props: { label: string; value: number }) {
  return (
    <div>
      <div className='text-muted-foreground text-xs'>{props.label}</div>
      <div className='text-lg font-semibold'>{props.value}</div>
    </div>
  )
}

function Field(props: { label: string; children: ReactNode }) {
  return (
    <div className='space-y-1.5'>
      <Label>{props.label}</Label>
      {props.children}
    </div>
  )
}

function ToggleLine(props: {
  label: string
  checked: boolean
  onCheckedChange: (checked: boolean) => void
}) {
  return (
    <div className='flex items-center justify-between gap-2 rounded-lg border px-3 py-2'>
      <Label className='text-sm'>{props.label}</Label>
      <Switch
        size='sm'
        checked={props.checked}
        onCheckedChange={props.onCheckedChange}
      />
    </div>
  )
}
