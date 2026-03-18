import { createFileRoute } from '@tanstack/react-router'
import { useState } from 'react'
import { BarChart3, TrendingUp, Zap, Clock, ArrowUpRight, ArrowDownRight } from 'lucide-react'
import { requireAuth, getAuthSession } from '../lib/auth'
import { Skeleton } from '#/components/ui/skeleton'
import { useUsageSummary, useUsageTop, useUsageTrends } from '../lib/api/hooks'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '#/components/ui/card'

import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '#/components/ui/table'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '#/components/ui/select'
import type { UsageSummaryRow, UsageTrendRow } from '../lib/api/types'

export const Route = createFileRoute('/usage')({
  beforeLoad: requireAuth,
  loader: async () => {
    const session = await getAuthSession()
    return { isAdmin: session.user?.is_admin ?? false }
  },
  component: UsagePage,
})

type TimeRange = '24h' | '7d' | '30d'

function getTimeRange(range: TimeRange): { start: string; end: string } {
  const end = new Date()
  const start = new Date()
  switch (range) {
    case '24h':
      start.setHours(start.getHours() - 24)
      break
    case '7d':
      start.setDate(start.getDate() - 7)
      break
    case '30d':
      start.setDate(start.getDate() - 30)
      break
  }
  // Truncate to the minute so the query key stays stable within the same minute,
  // allowing React Query to cache hits on re-navigation.
  end.setSeconds(0, 0)
  start.setSeconds(0, 0)
  return { start: start.toISOString(), end: end.toISOString() }
}

const CHART_COLORS = [
  '#6366f1', // indigo
  '#f59e0b', // amber
  '#10b981', // emerald
  '#ef4444', // red
  '#3b82f6', // blue
  '#8b5cf6', // violet
  '#f97316', // orange
  '#14b8a6', // teal
  '#ec4899', // pink
  '#84cc16', // lime
]

function formatNumber(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`
  return String(n)
}

function UsagePage() {
  const { isAdmin } = Route.useLoaderData()
  const [timeRange, setTimeRange] = useState<TimeRange>('24h')
  const [topDimension, setTopDimension] = useState('model')
  const [trendDimension, setTrendDimension] = useState('all')

  const { start, end } = getTimeRange(timeRange)
  const granularity = timeRange === '24h' ? 'hourly' : 'daily'

  const { data: summary, isLoading: summaryLoading } = useUsageSummary({ start, end })
  const { data: topData, isLoading: topLoading } = useUsageTop({ start, end, dimension: topDimension, limit: 10 })
  const { data: trends, isLoading: trendsLoading } = useUsageTrends({ start, end, granularity, dimension: trendDimension === 'all' ? undefined : trendDimension })


  const totalInput = summary?.data.reduce((sum, r) => sum + r.input_tokens, 0) ?? 0
  const totalOutput = summary?.data.reduce((sum, r) => sum + r.output_tokens, 0) ?? 0
  const totalTokens = summary?.data.reduce((sum, r) => sum + r.total_tokens, 0) ?? 0
  const totalRequests = summary?.data.reduce((sum, r) => sum + r.request_count, 0) ?? 0

  return (
    <div className="h-full overflow-auto bg-background py-8">
      <div className="container mx-auto max-w-[1400px] px-6">
        {/* Header */}
        <div className="mb-8 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <BarChart3 className="h-6 w-6 text-muted-foreground" />
            <h1 className="text-2xl font-medium tracking-tight">Token Usage</h1>
          </div>
          <Select value={timeRange} onValueChange={(v) => setTimeRange(v as TimeRange)}>
            <SelectTrigger className="w-[140px]">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="24h">Last 24 hours</SelectItem>
              <SelectItem value="7d">Last 7 days</SelectItem>
              <SelectItem value="30d">Last 30 days</SelectItem>
            </SelectContent>
          </Select>
        </div>

        {/* Stat Cards */}
        <div className="mb-6 grid gap-4 md:grid-cols-2 lg:grid-cols-4">
          <StatCard
            title="Total Tokens"
            value={formatNumber(totalTokens)}
            icon={Zap}
            description={`${formatNumber(totalInput)} in / ${formatNumber(totalOutput)} out`}
            isLoading={summaryLoading}
          />
          <StatCard
            title="Input Tokens"
            value={formatNumber(totalInput)}
            icon={ArrowUpRight}
            description="Prompt tokens consumed"
            isLoading={summaryLoading}
          />
          <StatCard
            title="Output Tokens"
            value={formatNumber(totalOutput)}
            icon={ArrowDownRight}
            description="Completion tokens generated"
            isLoading={summaryLoading}
          />
          <StatCard
            title="Requests"
            value={formatNumber(totalRequests)}
            icon={Clock}
            description={`Avg ${totalRequests > 0 ? formatNumber(Math.round(totalTokens / totalRequests)) : '0'} tokens/req`}
            isLoading={summaryLoading}
          />
        </div>

        {/* Trends Chart */}
        <Card className="mb-6">
          <CardHeader>
            <div className="flex items-center justify-between">
              <div>
                <CardTitle className="flex items-center gap-2 text-lg">
                  <TrendingUp className="h-5 w-5 text-muted-foreground" />
                  Usage Trends
                </CardTitle>
                <CardDescription>
                  Token consumption over time ({granularity})
                </CardDescription>
              </div>
              <Select value={trendDimension} onValueChange={setTrendDimension}>
                <SelectTrigger className="w-[140px]">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All (combined)</SelectItem>
                  <SelectItem value="model">By Model</SelectItem>
                  {isAdmin && <SelectItem value="user_sub">By User</SelectItem>}
                  <SelectItem value="provider">By Provider</SelectItem>
                  {isAdmin && <SelectItem value="tenant_id">By Tenant</SelectItem>}
                </SelectContent>
              </Select>
            </div>
          </CardHeader>
          <CardContent>
            {trendsLoading ? (
              <div className="space-y-2">
                <div className="flex justify-between">
                  <Skeleton className="h-3 w-10" />
                  <Skeleton className="h-3 w-10" />
                  <Skeleton className="h-3 w-6" />
                </div>
                <div className="flex items-end gap-px" style={{ height: '200px' }}>
                  {Array.from({ length: 24 }).map((_, i) => (
                    <div
                      key={i}
                      className="flex flex-1 flex-col items-stretch justify-end"
                    >
                      <Skeleton
                        className="w-full rounded-t-sm"
                        style={{ height: `${20 + Math.sin(i * 0.7) * 15 + Math.cos(i * 0.4) * 10 + 30}%` }}
                      />
                    </div>
                  ))}
                </div>
                <div className="flex justify-between">
                  <Skeleton className="h-3 w-10" />
                  <Skeleton className="h-3 w-10" />
                  <Skeleton className="h-3 w-10" />
                </div>
                <div className="flex items-center justify-center gap-4 pt-2">
                  <Skeleton className="h-3 w-14" />
                  <Skeleton className="h-3 w-16" />
                </div>
              </div>
            ) : trends && trends.data.length > 0 ? (
              <TrendsChart data={trends.data} granularity={granularity} dimension={trendDimension} />
            ) : (
              <div className="flex items-center justify-center py-12 text-muted-foreground">
                No trend data for this period
              </div>
            )}
          </CardContent>
        </Card>

        {/* Bottom Section: Top Consumers + Summary Table */}
        <div className="grid gap-6 lg:grid-cols-2">
          {/* Top Consumers */}
          <Card>
            <CardHeader>
              <div className="flex items-center justify-between">
                <CardTitle className="text-lg">Top Consumers</CardTitle>
                <Select value={topDimension} onValueChange={setTopDimension}>
                  <SelectTrigger className="w-[140px]">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="model">By Model</SelectItem>
                    {isAdmin && <SelectItem value="user_sub">By User</SelectItem>}
                    <SelectItem value="provider">By Provider</SelectItem>
                    {isAdmin && <SelectItem value="tenant_id">By Tenant</SelectItem>}
                  </SelectContent>
                </Select>
              </div>
            </CardHeader>
            <CardContent>
              {topLoading ? (
                <div className="space-y-3">
                  {Array.from({ length: 5 }).map((_, i) => (
                    <div key={i} className="space-y-1">
                      <div className="flex items-center justify-between text-sm">
                        <div className="flex items-center gap-2">
                          <Skeleton className="h-3 w-5" />
                          <Skeleton className="h-4 w-32" />
                        </div>
                        <Skeleton className="h-4 w-12" />
                      </div>
                      <div className="h-2 rounded-full bg-muted">
                        <Skeleton className="h-2 rounded-full" style={{ width: `${80 - i * 12}%` }} />
                      </div>
                    </div>
                  ))}
                </div>
              ) : topData && topData.data.length > 0 ? (
                <div className="space-y-3">
                  {topData.data.map((row, i) => {
                    const maxTokens = topData.data[0].total_tokens
                    const pct = maxTokens > 0 ? (row.total_tokens / maxTokens) * 100 : 0
                    return (
                      <div key={row.key} className="space-y-1">
                        <div className="flex items-center justify-between text-sm">
                          <span className="flex items-center gap-2 truncate font-medium">
                            <span className="text-xs text-muted-foreground">#{i + 1}</span>
                            <span className="truncate font-mono">{row.key}</span>
                          </span>
                          <span className="ml-2 shrink-0 text-muted-foreground">
                            {formatNumber(row.total_tokens)}
                          </span>
                        </div>
                        <div className="h-2 rounded-full bg-muted">
                          <div
                            className="h-2 rounded-full"
                            style={{ width: `${pct}%`, backgroundColor: CHART_COLORS[i % CHART_COLORS.length] }}
                          />
                        </div>
                      </div>
                    )
                  })}
                </div>
              ) : (
                <div className="flex items-center justify-center py-12 text-muted-foreground">
                  No data for this period
                </div>
              )}
            </CardContent>
          </Card>

          {/* Summary Breakdown */}
          <Card>
            <CardHeader>
              <CardTitle className="text-lg">Breakdown by Model</CardTitle>
              <CardDescription>
                Aggregated usage per model in this period
              </CardDescription>
            </CardHeader>
            <CardContent>
              {summaryLoading ? (
                <div>
                  <div className="flex items-center gap-4 border-b pb-3 mb-1">
                    <Skeleton className="h-3 w-1/4" />
                    <Skeleton className="h-3 w-12 ml-auto" />
                    <Skeleton className="h-3 w-12" />
                    <Skeleton className="h-3 w-12" />
                    <Skeleton className="h-3 w-10" />
                  </div>
                  {Array.from({ length: 5 }).map((_, i) => (
                    <div key={i} className="flex items-center gap-4 py-3 border-b last:border-0">
                      <Skeleton className="h-4 w-1/3" />
                      <Skeleton className="h-4 w-12 ml-auto" />
                      <Skeleton className="h-4 w-12" />
                      <Skeleton className="h-4 w-12" />
                      <Skeleton className="h-4 w-10" />
                    </div>
                  ))}
                </div>
              ) : summary && summary.data.length > 0 ? (
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Model</TableHead>
                      <TableHead className="text-right">Input</TableHead>
                      <TableHead className="text-right">Output</TableHead>
                      <TableHead className="text-right">Total</TableHead>
                      <TableHead className="text-right">Reqs</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {summary.data.map((row: UsageSummaryRow) => (
                      <TableRow key={`${row.tenant_id}-${row.user_sub}-${row.provider}-${row.model}`}>
                        <TableCell className="font-mono text-sm">{row.model || '—'}</TableCell>
                        <TableCell className="text-right text-sm">{formatNumber(row.input_tokens)}</TableCell>
                        <TableCell className="text-right text-sm">{formatNumber(row.output_tokens)}</TableCell>
                        <TableCell className="text-right text-sm font-medium">{formatNumber(row.total_tokens)}</TableCell>
                        <TableCell className="text-right text-sm text-muted-foreground">{formatNumber(row.request_count)}</TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              ) : (
                <div className="flex items-center justify-center py-12 text-muted-foreground">
                  No summary data for this period
                </div>
              )}
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  )
}

function StatCard({
  title,
  value,
  icon: Icon,
  description,
  isLoading = false,
}: {
  title: string
  value: string
  icon: any
  description: string
  isLoading?: boolean
}) {
  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between pb-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">{title}</CardTitle>
        <Icon className="h-4 w-4 text-muted-foreground" />
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <div className="space-y-2">
            <Skeleton className="h-8 w-24" />
            <Skeleton className="h-3 w-32" />
          </div>
        ) : (
          <>
            <div className="text-2xl font-bold">{value}</div>
            <p className="text-xs text-muted-foreground">{description}</p>
          </>
        )}
      </CardContent>
    </Card>
  )
}

function TrendsChart({ data, granularity, dimension }: { data: UsageTrendRow[]; granularity: string; dimension?: string }) {
  const isGrouped = !!dimension && dimension !== 'all' && data.some((d) => d.key != null)

  // Collect unique ordered buckets and keys
  const buckets = [...new Set(data.map((d) => d.bucket))].sort()
  const keys = isGrouped
    ? [...new Set(data.map((d) => d.key ?? ''))].filter(Boolean)
    : []

  // For each bucket sum total tokens (across all keys) to find the max bar height
  const bucketTotals = buckets.map((b) =>
    data.filter((d) => d.bucket === b).reduce((s, d) => s + d.total_tokens, 0),
  )
  const maxTotal = Math.max(...bucketTotals, 1)

  function formatBucketLabel(bucket: string) {
    const date = new Date(bucket)
    return granularity === 'hourly'
      ? date.toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
      : date.toLocaleDateString([], { month: 'short', day: 'numeric' })
  }

  return (
    <div className="space-y-2">
      {/* Y-axis labels */}
      <div className="flex items-end justify-between text-xs text-muted-foreground">
        <span>{formatNumber(maxTotal)}</span>
        <span>{formatNumber(Math.round(maxTotal / 2))}</span>
        <span>0</span>
      </div>

      {/* Bar chart */}
      <div className="flex items-end gap-px" style={{ height: '200px' }}>
        {isGrouped
          ? buckets.map((bucket, i) => {
              const entries = data.filter((d) => d.bucket === bucket)
              const bucketTotal = bucketTotals[i]
              const label = formatBucketLabel(bucket)
              const tooltipLines = entries
                .map((e) => `${e.key ?? ''}: ${formatNumber(e.total_tokens)}`)
                .join('\n')
              return (
                <div
                  key={bucket}
                  className="group relative flex flex-1 flex-col items-stretch justify-end"
                  title={`${label}\n${tooltipLines}`}
                  style={{ height: '200px' }}
                >
                  <div className="flex w-full flex-col items-stretch justify-end" style={{ height: `${(bucketTotal / maxTotal) * 200}px` }}>
                    {keys.map((key, ki) => {
                      const entry = entries.find((e) => e.key === key)
                      if (!entry) return null
                      const segH = (entry.total_tokens / maxTotal) * 200
                      return (
                        <div
                          key={key}
                          className="w-full"
                          style={{
                            height: `${segH}px`,
                            backgroundColor: CHART_COLORS[ki % CHART_COLORS.length],
                          }}
                        />
                      )
                    })}
                  </div>
                </div>
              )
            })
          : data.map((row, i) => {
              const inputPct = maxTotal > 0 ? (row.input_tokens / maxTotal) * 100 : 0
              const outputPct = maxTotal > 0 ? (row.output_tokens / maxTotal) * 100 : 0
              const label = formatBucketLabel(row.bucket)
              return (
                <div
                  key={i}
                  className="group relative flex flex-1 flex-col items-center justify-end"
                  title={`${label}\nInput: ${formatNumber(row.input_tokens)}\nOutput: ${formatNumber(row.output_tokens)}\nTotal: ${formatNumber(row.total_tokens)}\nRequests: ${formatNumber(row.request_count)}`}
                >
                  <div className="flex w-full flex-col items-stretch">
                    <div className="w-full rounded-t-sm bg-primary/50" style={{ height: `${(outputPct / 100) * 200}px` }} />
                    <div className="w-full bg-primary" style={{ height: `${(inputPct / 100) * 200}px` }} />
                  </div>
                </div>
              )
            })}
      </div>

      {/* X-axis labels */}
      <div className="flex justify-between text-xs text-muted-foreground">
        {buckets.length > 0 && (
          <>
            <span>{formatBucketLabel(buckets[0])}</span>
            {buckets.length > 2 && <span>{formatBucketLabel(buckets[Math.floor(buckets.length / 2)])}</span>}
            <span>{formatBucketLabel(buckets[buckets.length - 1])}</span>
          </>
        )}
      </div>

      {/* Legend */}
      <div className="flex flex-wrap items-center justify-center gap-4 pt-2 text-xs text-muted-foreground">
        {isGrouped
          ? keys.map((key, ki) => (
              <span key={key} className="flex items-center gap-1">
                <span
                  className="inline-block h-2.5 w-2.5 rounded-sm"
                  style={{ backgroundColor: CHART_COLORS[ki % CHART_COLORS.length] }}
                />
                {key}
              </span>
            ))
          : (
            <>
              <span className="flex items-center gap-1">
                <span className="inline-block h-2.5 w-2.5 rounded-sm bg-primary" />
                Input
              </span>
              <span className="flex items-center gap-1">
                <span className="inline-block h-2.5 w-2.5 rounded-sm bg-primary/50" />
                Output
              </span>
            </>
          )}
      </div>
    </div>
  )
}
