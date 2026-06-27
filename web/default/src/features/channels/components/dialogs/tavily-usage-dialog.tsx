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
import { RefreshCw, RotateCcw, Save } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatTimestampToDate } from '@/lib/format'

import type { TavilyUsageResponse } from '../../api'
import { CHANNEL_STATUS_CONFIG } from '../../constants'

export type TavilyUsageDialogData = TavilyUsageResponse

const EMPTY_USAGE_ROWS: NonNullable<TavilyUsageResponse['data']> = []

type TavilyUsageDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  channelName: string
  response: TavilyUsageDialogData | null
  onRefresh: () => Promise<void>
  onReset: (keyIndex?: number) => Promise<void>
  onSync: (keyIndex?: number) => Promise<void>
  onUpdate: (
    keyIndex: number,
    params: { monthly_limit_credits: number; project_id: string }
  ) => Promise<void>
  title?: string
  unitLabel?: string
  projectLabel?: string
  isRefreshing?: boolean
  isResetting?: boolean
  isSyncing?: boolean
  isSaving?: boolean
}

function usagePercent(used: number, limit: number) {
  if (limit <= 0) {
    return 0
  }
  return Math.max(0, Math.min(100, (used / limit) * 100))
}

export function TavilyUsageDialog({
  open,
  onOpenChange,
  channelName,
  response,
  onRefresh,
  onReset,
  onSync,
  onUpdate,
  title,
  unitLabel,
  projectLabel,
  isRefreshing = false,
  isResetting = false,
  isSyncing = false,
  isSaving = false,
}: TavilyUsageDialogProps) {
  const { t } = useTranslation()
  const rows = response?.data ?? EMPTY_USAGE_ROWS
  const dialogTitle = title ?? t('Tavily Usage')
  const usageUnitLabel = unitLabel ?? t('Credits')
  const projectColumnLabel = projectLabel ?? t('Project')
  const [drafts, setDrafts] = useState<
    Record<number, { monthlyLimit: string; projectId: string }>
  >({})
  const isBusy = isRefreshing || isResetting || isSyncing || isSaving

  useEffect(() => {
    const nextDrafts: Record<number, { monthlyLimit: string; projectId: string }> = {}
    for (const row of rows) {
      nextDrafts[row.key_index] = {
        monthlyLimit: String(row.monthly_limit_credits || ''),
        projectId: row.project_id || '',
      }
    }
    setDrafts(nextDrafts)
  }, [rows])

  const setDraft = (
    keyIndex: number,
    patch: Partial<{ monthlyLimit: string; projectId: string }>
  ) => {
    setDrafts((current) => ({
      ...current,
      [keyIndex]: {
        monthlyLimit: current[keyIndex]?.monthlyLimit ?? '',
        projectId: current[keyIndex]?.projectId ?? '',
        ...patch,
      },
    }))
  }

  const saveDraft = async (keyIndex: number) => {
    const draft = drafts[keyIndex]
    const monthlyLimit = Number.parseInt(draft?.monthlyLimit || '', 10)
    if (!Number.isFinite(monthlyLimit) || monthlyLimit <= 0) {
      return
    }
    await onUpdate(keyIndex, {
      monthly_limit_credits: monthlyLimit,
      project_id: draft?.projectId || '',
    })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-4xl'>
        <DialogHeader>
          <DialogTitle>{dialogTitle}</DialogTitle>
          <DialogDescription>{channelName}</DialogDescription>
        </DialogHeader>

        <div className='max-h-[60vh] overflow-auto'>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('Key')}</TableHead>
                <TableHead>{projectColumnLabel}</TableHead>
                <TableHead>{t('Status')}</TableHead>
                <TableHead>
                  {t('Used')} ({usageUnitLabel})
                </TableHead>
                <TableHead>
                  {t('Remaining')} ({usageUnitLabel})
                </TableHead>
                <TableHead>{t('Synced')}</TableHead>
                <TableHead>{t('Reset At')}</TableHead>
                <TableHead>{t('Error')}</TableHead>
                <TableHead className='text-right'>{t('Actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.length === 0 ? (
                <TableRow>
                  <TableCell
                    colSpan={9}
                    className='text-muted-foreground h-24 text-center'
                  >
                    {t('No data')}
                  </TableCell>
                </TableRow>
              ) : (
                rows.map((item) => {
                  const status =
                    CHANNEL_STATUS_CONFIG[
                      item.status as keyof typeof CHANNEL_STATUS_CONFIG
                    ] || CHANNEL_STATUS_CONFIG[0]
                  const percent = usagePercent(
                    item.used_credits,
                    item.monthly_limit_credits
                  )

                  return (
                    <TableRow key={item.key_index}>
                      <TableCell>
                        <div className='flex flex-col gap-1'>
                          <span className='font-medium'>
                            #{item.key_index}
                          </span>
                          <span className='text-muted-foreground font-mono text-xs'>
                            {item.key_tail || '-'}
                          </span>
                        </div>
                      </TableCell>
                      <TableCell>
                        <Input
                          value={drafts[item.key_index]?.projectId ?? ''}
                          onChange={(event) =>
                            setDraft(item.key_index, {
                              projectId: event.target.value,
                            })
                          }
                          className='h-8 min-w-32'
                          placeholder={t('Project')}
                          disabled={isBusy}
                        />
                      </TableCell>
                      <TableCell>
                        <StatusBadge
                          label={t(status.label)}
                          variant={status.variant}
                          size='sm'
                          copyable={false}
                        />
                      </TableCell>
                      <TableCell>
                        <div className='min-w-32 space-y-1'>
                          <div className='tabular-nums'>
                            {item.used_credits} /{' '}
                            <Input
                              type='number'
                              min={1}
                              value={
                                drafts[item.key_index]?.monthlyLimit ?? ''
                              }
                              onChange={(event) =>
                                setDraft(item.key_index, {
                                  monthlyLimit: event.target.value,
                                })
                              }
                              className='ml-1 inline-flex h-7 w-20 px-2'
                              disabled={isBusy}
                            />
                          </div>
                          <div className='bg-muted h-1 overflow-hidden rounded-full'>
                            <div
                              className='bg-primary h-full'
                              style={{ width: `${percent}%` }}
                            />
                          </div>
                        </div>
                      </TableCell>
                      <TableCell className='tabular-nums'>
                        {item.remaining_credits}
                      </TableCell>
                      <TableCell>
                        {formatTimestampToDate(item.last_sync_at)}
                      </TableCell>
                      <TableCell>
                        {formatTimestampToDate(item.reset_at)}
                      </TableCell>
                      <TableCell className='max-w-48 truncate'>
                        {item.last_error || '-'}
                      </TableCell>
                      <TableCell>
                        <div className='flex justify-end gap-1'>
                          <Button
                            variant='outline'
                            size='icon-sm'
                            disabled={isBusy}
                            title={t('Save')}
                            onClick={() => saveDraft(item.key_index)}
                          >
                            <Save />
                          </Button>
                          <Button
                            variant='outline'
                            size='icon-sm'
                            disabled={isBusy}
                            title={t('Sync')}
                            onClick={() => onSync(item.key_index)}
                          >
                            <RefreshCw
                              className={isSyncing ? 'animate-spin' : ''}
                            />
                          </Button>
                          <Button
                            variant='outline'
                            size='icon-sm'
                            disabled={isBusy}
                            title={t('Reset')}
                            onClick={() => onReset(item.key_index)}
                          >
                            <RotateCcw />
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  )
                })
              )}
            </TableBody>
          </Table>
        </div>

        <DialogFooter>
          <Button variant='outline' disabled={isBusy} onClick={onRefresh}>
            <RefreshCw className={isRefreshing ? 'animate-spin' : ''} />
            {t('Refresh')}
          </Button>
          <Button
            variant='outline'
            disabled={isBusy || rows.length === 0}
            onClick={() => onSync()}
          >
            <RefreshCw className={isSyncing ? 'animate-spin' : ''} />
            {t('Sync All')}
          </Button>
          <Button disabled={isBusy || rows.length === 0} onClick={() => onReset()}>
            <RotateCcw />
            {t('Reset All')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
