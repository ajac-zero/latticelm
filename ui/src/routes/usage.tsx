import { createFileRoute } from '@tanstack/react-router'
import { useState } from 'react'
import { BarChart3, TrendingUp, Zap, Clock, ArrowUpRight, ArrowDownRight } from 'lucide-react'
import { requireAuth, getAuthSession } from '../lib/auth'
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
  return { start: start.toISOString(), end: end.toISOString() }
}

function formatNumber(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`
  return String(n)
}

function UsagePage() {
  const { isAdmin } = Route.useLoaderData()
  const [timeRange, setTimeRange] = useState<TimeRange>('24h')
  const [topDimension, setTopDimension] = useState('model')

  const { start, end } = getTimeRange(timeRange)
  const granularity = timeRange === '24h' ? 'hourly' : 'daily'

  const { data: summary, isLoading: summaryLoading } = useUsageSummary({ start, end })
  const { data: topData, isLoading: topLoading } = useUsageTop({ start, end, dimension: topDimension, limit: 10 })
  const { data: trends, isLoading: trendsLoading } = useUsageTrends({ start, end, granularity })

  const totalInput = summary?.data.reduce((sum, r) => sum + r.input_tokens, 0) ?? 0
  const totalOutput = summary?.data.reduce((sum, r) => sum + r.output_tokens, 0) ?? 0
  const totalTokens = summary?.data.reduce((sum, r) => sum + r.total_tokens, 0) ?? 0
  const totalRequests = summary?.data.reduce((sum, r) => sum + r.request_count, 0) ?? 0

  const loading = summaryLoading || topLoading || trendsLoading

  return (
    <div className="min-h-screen bg-background py-8">
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

        {loading ? (
          <div className="flex min-h-[calc(100vh-12rem)] items-center justify-center">
            <div className="text-lg">Loading...</div>
          </div>
        ) : (
          <>
            {/* Stat Cards */}
            <div className="mb-6 grid gap-4 md:grid-cols-2 lg:grid-cols-4">
              <StatCard
                title="Total Tokens"
                value={formatNumber(totalTokens)}
                icon={Zap}
                description={`${formatNumber(totalInput)} in / ${formatNumber(totalOutput)} out`}
              />
              <StatCard
                title="Input Tokens"
                value={formatNumber(totalInput)}
                icon={ArrowUpRight}
                description="Prompt tokens consumed"
              />
              <StatCard
                title="Output Tokens"
                value={formatNumber(totalOutput)}
                icon={ArrowDownRight}
                description="Completion tokens generated"
              />
              <StatCard
                title="Requests"
                value={formatNumber(totalRequests)}
                icon={Clock}
                description={`Avg ${totalRequests > 0 ? formatNumber(Math.round(totalTokens / totalRequests)) : '0'} tokens/req`}
              />
            </div>

            {/* Trends Chart */}
            <Card className="mb-6">
              <CardHeader>
                <CardTitle className="flex items-center gap-2 text-lg">
                  <TrendingUp className="h-5 w-5 text-muted-foreground" />
                  Usage Trends
                </CardTitle>
                <CardDescription>
                  Token consumption over time ({granularity})
                </CardDescription>
              </CardHeader>
              <CardContent>
                {trends && trends.data.length > 0 ? (
                  <TrendsChart data={trends.data} granularity={granularity} />
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
                  {topData && topData.data.length > 0 ? (
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
                                className="h-2 rounded-full bg-primary/70"
                                style={{ width: `${pct}%` }}
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
                  {summary && summary.data.length > 0 ? (
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
          </>
        )}
      </div>
    </div>
  )
}

function StatCard({
  title,
  value,
  icon: Icon,
  description,
}: {
  title: string
  value: string
  icon: any
  description: string
}) {
  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between pb-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">{title}</CardTitle>
        <Icon className="h-4 w-4 text-muted-foreground" />
      </CardHeader>
      <CardContent>
        <div className="text-2xl font-bold">{value}</div>
        <p className="text-xs text-muted-foreground">{description}</p>
      </CardContent>
    </Card>
  )
}

function TrendsChart({ data, granularity }: { data: UsageTrendRow[]; granularity: string }) {
  const maxTotal = Math.max(...data.map((d) => d.total_tokens), 1)

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
        {data.map((row, i) => {
          const inputPct = maxTotal > 0 ? (row.input_tokens / maxTotal) * 100 : 0
          const outputPct = maxTotal > 0 ? (row.output_tokens / maxTotal) * 100 : 0
          const date = new Date(row.bucket)
          const label =
            granularity === 'hourly'
              ? date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
              : date.toLocaleDateString([], { month: 'short', day: 'numeric' })

          return (
            <div
              key={i}
              className="group relative flex flex-1 flex-col items-center justify-end"
              title={`${label}\nInput: ${formatNumber(row.input_tokens)}\nOutput: ${formatNumber(row.output_tokens)}\nTotal: ${formatNumber(row.total_tokens)}\nRequests: ${formatNumber(row.request_count)}`}
            >
              <div className="flex w-full flex-col items-stretch">
                <div
                  className="w-full rounded-t-sm bg-primary/50"
                  style={{ height: `${(outputPct / 100) * 200}px` }}
                />
                <div
                  className="w-full bg-primary"
                  style={{ height: `${(inputPct / 100) * 200}px` }}
                />
              </div>
            </div>
          )
        })}
      </div>

      {/* X-axis labels */}
      <div className="flex justify-between text-xs text-muted-foreground">
        {data.length > 0 && (
          <>
            <span>
              {granularity === 'hourly'
                ? new Date(data[0].bucket).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
                : new Date(data[0].bucket).toLocaleDateString([], { month: 'short', day: 'numeric' })}
            </span>
            {data.length > 2 && (
              <span>
                {granularity === 'hourly'
                  ? new Date(data[Math.floor(data.length / 2)].bucket).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
                  : new Date(data[Math.floor(data.length / 2)].bucket).toLocaleDateString([], { month: 'short', day: 'numeric' })}
              </span>
            )}
            <span>
              {granularity === 'hourly'
                ? new Date(data[data.length - 1].bucket).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
                : new Date(data[data.length - 1].bucket).toLocaleDateString([], { month: 'short', day: 'numeric' })}
            </span>
          </>
        )}
      </div>

      {/* Legend */}
      <div className="flex items-center justify-center gap-4 pt-2 text-xs text-muted-foreground">
        <span className="flex items-center gap-1">
          <span className="inline-block h-2.5 w-2.5 rounded-sm bg-primary" />
          Input
        </span>
        <span className="flex items-center gap-1">
          <span className="inline-block h-2.5 w-2.5 rounded-sm bg-primary/50" />
          Output
        </span>
      </div>
    </div>
  )
}
