import { createFileRoute, Link } from '@tanstack/react-router'
import { MessageSquare, ArrowRight, Clock, Activity, Database, Zap } from 'lucide-react'
import { requireAuth, getAuthSession } from '../lib/auth'
import {
  useConversations,
  useModels,
  useHealth,
  useSystemInfo,
  useProviders,
  useConfig,
  useUsageSummary,
} from '../lib/api/hooks'
import { Card, CardContent, CardHeader, CardTitle } from '#/components/ui/card'
import { Button } from '#/components/ui/button'

export const Route = createFileRoute('/')({
  beforeLoad: requireAuth,
  loader: async () => {
    const session = await getAuthSession()
    return {
      user: session.user,
      isAdmin: session.user?.is_admin ?? false,
    }
  },
  component: HomePage,
})

function getGreeting() {
  const hour = new Date().getHours()
  if (hour < 12) return 'Good morning'
  if (hour < 17) return 'Good afternoon'
  return 'Good evening'
}

function formatRelativeDate(iso: string) {
  const date = new Date(iso)
  const now = new Date()
  const diffDays = Math.floor((now.getTime() - date.getTime()) / (1000 * 60 * 60 * 24))
  if (diffDays === 0) return 'Today'
  if (diffDays === 1) return 'Yesterday'
  if (diffDays < 7) return `${diffDays}d ago`
  return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
}

function formatNumber(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`
  return String(n)
}

function HomePage() {
  const { user, isAdmin } = Route.useLoaderData()

  const weekAgo = new Date()
  weekAgo.setDate(weekAgo.getDate() - 7)
  const usageRange = { start: weekAgo.toISOString(), end: new Date().toISOString() }

  const { data: conversationsData, isLoading: convLoading } = useConversations()
  const { data: models = [] } = useModels()
  const { data: health } = useHealth(isAdmin)
  const { data: systemInfo } = useSystemInfo(isAdmin)
  const { data: providers = [] } = useProviders(isAdmin)
  const { data: config } = useConfig(isAdmin)
  const { data: usageSummary } = useUsageSummary(usageRange, isAdmin && (config?.usage?.enabled ?? false))

  const displayName = user?.name || user?.email || 'there'
  const recentConversations = conversationsData?.conversations?.slice(0, 5) ?? []

  const totalRequests = usageSummary?.data?.reduce((sum, row) => sum + row.request_count, 0) ?? 0
  const totalTokens = usageSummary?.data?.reduce((sum, row) => sum + row.total_tokens, 0) ?? 0
  const activeProviders = providers.filter(p => p.status === 'active').length

  return (
    <div className="min-h-screen bg-background py-8">
      <div className="container mx-auto max-w-5xl px-6">
        {/* Greeting */}
        <div className="mb-8">
          <h1 className="text-2xl font-semibold tracking-tight">
            {getGreeting()}, {displayName}
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            {isAdmin ? 'Here\'s an overview of your gateway.' : 'Welcome back.'}
          </p>
        </div>

        {/* Admin stats row */}
        {isAdmin && (
          <div className="mb-6 grid gap-4 sm:grid-cols-3">
            <Card>
              <CardHeader className="pb-2">
                <div className="flex items-center gap-2">
                  <Activity className="h-4 w-4 text-muted-foreground" />
                  <CardTitle className="text-sm font-medium text-muted-foreground">System Health</CardTitle>
                </div>
              </CardHeader>
              <CardContent>
                <div className="flex items-center gap-2">
                  <span
                    className={`h-2 w-2 rounded-full ${
                      health?.status === 'healthy'
                        ? 'bg-green-500'
                        : health?.status === 'degraded'
                          ? 'bg-yellow-500'
                          : 'bg-muted'
                    }`}
                  />
                  <span className="text-sm capitalize">{health?.status ?? '—'}</span>
                </div>
                {systemInfo?.uptime && (
                  <p className="mt-1 text-xs text-muted-foreground">Uptime: {systemInfo.uptime}</p>
                )}
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="pb-2">
                <div className="flex items-center gap-2">
                  <Database className="h-4 w-4 text-muted-foreground" />
                  <CardTitle className="text-sm font-medium text-muted-foreground">Providers</CardTitle>
                </div>
              </CardHeader>
              <CardContent>
                <p className="text-2xl font-semibold">{providers.length}</p>
                <p className="text-xs text-muted-foreground">
                  {activeProviders} active
                </p>
              </CardContent>
            </Card>

            {config?.usage?.enabled ? (
              <Card>
                <CardHeader className="pb-2">
                  <div className="flex items-center gap-2">
                    <Zap className="h-4 w-4 text-muted-foreground" />
                    <CardTitle className="text-sm font-medium text-muted-foreground">Requests (7d)</CardTitle>
                  </div>
                </CardHeader>
                <CardContent>
                  <p className="text-2xl font-semibold">{formatNumber(totalRequests)}</p>
                  <p className="text-xs text-muted-foreground">{formatNumber(totalTokens)} tokens</p>
                </CardContent>
              </Card>
            ) : (
              <Card>
                <CardHeader className="pb-2">
                  <div className="flex items-center gap-2">
                    <Zap className="h-4 w-4 text-muted-foreground" />
                    <CardTitle className="text-sm font-medium text-muted-foreground">Models</CardTitle>
                  </div>
                </CardHeader>
                <CardContent>
                  <p className="text-2xl font-semibold">{models.length}</p>
                  <p className="text-xs text-muted-foreground">available</p>
                </CardContent>
              </Card>
            )}
          </div>
        )}

        {/* Main content grid */}
        <div className="grid gap-6 md:grid-cols-2">
          {/* Playground CTA */}
          <Card className="border-primary/20 bg-gradient-to-br from-primary/5 to-transparent">
            <CardHeader>
              <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-primary/10">
                <MessageSquare className="h-4 w-4 text-primary" />
              </div>
              <CardTitle className="mt-3 text-lg">Playground</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <p className="text-sm text-muted-foreground">
                Chat with any available model.
                {models.length > 0
                  ? ` ${models.length} model${models.length !== 1 ? 's' : ''} available.`
                  : ''}
              </p>
              <Button asChild size="sm">
                <Link to="/playground">
                  Open Playground <ArrowRight className="ml-1.5 h-3.5 w-3.5" />
                </Link>
              </Button>
            </CardContent>
          </Card>

          {/* Recent Conversations */}
          <Card>
            <CardHeader>
              <div className="flex items-center justify-between">
                <CardTitle className="text-lg">Recent Conversations</CardTitle>
                <Link to="/playground" className="text-xs text-muted-foreground hover:text-foreground transition-colors">
                  Go to Playground
                </Link>
              </div>
            </CardHeader>
            <CardContent>
              {convLoading ? (
                <p className="text-sm text-muted-foreground">Loading...</p>
              ) : recentConversations.length === 0 ? (
                <p className="text-sm text-muted-foreground">
                  No conversations yet. Start one in the Playground.
                </p>
              ) : (
                <ul className="space-y-3">
                  {recentConversations.map(c => (
                    <li key={c.id} className="flex items-start justify-between gap-3">
                      <div className="min-w-0">
                        <p className="truncate font-mono text-xs text-foreground">{c.model}</p>
                        <p className="text-xs text-muted-foreground">
                          {c.message_count} message{c.message_count !== 1 ? 's' : ''}
                        </p>
                      </div>
                      <div className="flex shrink-0 items-center gap-1 text-xs text-muted-foreground">
                        <Clock className="h-3 w-3" />
                        {formatRelativeDate(c.updated_at)}
                      </div>
                    </li>
                  ))}
                </ul>
              )}
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  )
}
