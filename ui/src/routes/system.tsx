import { createFileRoute } from '@tanstack/react-router'
import { useState } from 'react'
import { Activity, Server, Database, Lock, Settings, BarChart3 } from 'lucide-react'
import { useSystemInfo, useHealth, useConfig, useProviders } from '../lib/api/hooks'
import { requireAdmin } from '../lib/auth'

export const Route = createFileRoute('/system')({
  beforeLoad: requireAdmin,
  component: DashboardPage,
})

function DashboardPage() {
  const { data: systemInfo, isLoading: systemLoading } = useSystemInfo()
  const { data: health, isLoading: healthLoading } = useHealth()
  const { data: config, isLoading: configLoading } = useConfig()
  const { data: providers, isLoading: providersLoading } = useProviders()

  const loading = systemLoading || healthLoading || configLoading || providersLoading

  if (loading) {
    return (
      <div className="flex min-h-[calc(100vh-5rem)] items-center justify-center">
        <div className="text-lg">Loading...</div>
      </div>
    )
  }

  const getServerAddress = () => {
    if (!config) return 'N/A'
    const addr = config.server.Address || config.server.address
    return addr || ':8080'
  }

  const getMaxRequestSize = () => {
    if (!config) return 'N/A'
    const size = config.server.MaxRequestBodySize || config.server.max_request_body_size
    if (!size || isNaN(size)) return '10 MB'
    return `${(size / 1024 / 1024).toFixed(0)} MB`
  }

  const isConversationsEnabled = () => {
    if (!config) return false
    const convConfig = config.conversations as any
    return convConfig.Enabled === true || !!(convConfig.Store || convConfig.store)
  }

  const getConversationStore = () => {
    if (!config) return 'N/A'
    const convConfig = config.conversations as any
    return convConfig.Store || convConfig.store || 'N/A'
  }

  const getConversationDriver = () => {
    if (!config) return 'N/A'
    const convConfig = config.conversations as any
    return convConfig.Driver || convConfig.driver || 'N/A'
  }

  const getConversationTTL = () => {
    if (!config) return 'N/A'
    const convConfig = config.conversations as any
    return convConfig.TTL || convConfig.ttl || 'N/A'
  }

  const getMaxConnections = () => {
    if (!config) return ''
    const convConfig = config.conversations as any
    const maxConns = convConfig.MaxOpenConns || convConfig.max_open_conns
    return maxConns ? String(maxConns) : ''
  }

  const getProviderStatus = (providerName: string) => {
    const provider = providers?.find(p => p.name === providerName)
    return provider?.status || 'active'
  }

  const getProviderModels = (providerName: string) => {
    if (!config) return []
    return config.models
      .filter(m => {
        const modelProvider = m.Provider || m.provider
        return modelProvider === providerName
      })
      .map(m => m.Name || m.name || '')
  }

  return (
    <div className="min-h-screen bg-background py-8">
      <div className="container mx-auto max-w-[1400px] px-6">
        {/* Top Grid */}
        <div className="mb-6 grid gap-6 md:grid-cols-2">
          {/* Server Configuration */}
          <InfoCard title="Server Configuration" icon={Server}>
            <div className="space-y-4">
              <InfoRow label="Address" value={getServerAddress()} mono />
              <InfoRow label="Max Request Size" value={getMaxRequestSize()} />
              <InfoRow label="Platform" value={systemInfo?.platform || 'darwin/arm64'} />
              <InfoRow label="Go Version" value={systemInfo?.go_version || 'N/A'} />
              <InfoRow label="Uptime" value={systemInfo?.uptime || 'N/A'} />
            </div>
          </InfoCard>

          {/* Health Status */}
          <InfoCard title="Health Status" icon={Activity}>
            {health && (
              <div>
                <div className="mb-4 flex items-center justify-between border-b border-border pb-4">
                  <span className="text-sm text-muted-foreground">Overall Status:</span>
                  <StatusBadge status={health.status} />
                </div>
                <div className="space-y-3">
                  <HealthItem
                    label="Conversation Store"
                    status={health.checks.conversation_store?.status || 'unknown'}
                    description={health.checks.conversation_store?.message || 'PostgreSQL connected'}
                  />
                  <HealthItem
                    label="Providers"
                    status={health.checks.providers?.status || 'healthy'}
                    description={`${config ? Object.keys(config.providers).length : 0} provider(s) configured`}
                  />
                  <HealthItem
                    label="Server"
                    status={health.checks.server?.status || 'healthy'}
                    description={health.checks.server?.message || 'Server is running'}
                  />
                </div>
              </div>
            )}
          </InfoCard>
        </div>

        {/* Providers Section */}
        <div className="mb-6 rounded-xl border border-border bg-card p-6">
          <div className="mb-6 flex items-center gap-2">
            <Database className="h-5 w-5 text-muted-foreground" />
            <h2 className="text-lg font-medium tracking-tight">Providers</h2>
          </div>
          {config && config.providers && Object.keys(config.providers).length > 0 ? (
            <div className="space-y-4">
              {Object.entries(config.providers).map(([name, providerConfig]) => (
                <ProviderCard
                  key={name}
                  name={name}
                  status={getProviderStatus(name)}
                  type={providerConfig.type}
                  endpoint={providerConfig.endpoint}
                  apiKey={providerConfig.api_key}
                  modelsCount={getProviderModels(name).length}
                  models={getProviderModels(name)}
                />
              ))}
            </div>
          ) : (
            <div className="py-8 text-center text-muted-foreground">No providers configured</div>
          )}
        </div>

        {/* Bottom Grid */}
        <div className="mb-6 grid gap-6 md:grid-cols-3">
          {/* Authentication */}
          <InfoCard title="Authentication" icon={Lock}>
            {config && (
              <div className="space-y-4">
                <div className="flex items-center justify-between">
                  <span className="text-sm text-muted-foreground">Status:</span>
                  <StatusBadge status={config.auth?.enabled ? 'active' : 'disabled'} />
                </div>
                {config.auth?.enabled ? (
                  <>
                    <InfoRow label="Issuer" value={config.auth.issuer || 'Not configured'} />
                    <InfoRow label="Audience" value={config.auth.audience || 'Not configured'} />
                  </>
                ) : (
                  <p className="text-xs text-muted-foreground">Authentication is currently disabled</p>
                )}
              </div>
            )}
          </InfoCard>

          {/* Conversations */}
          <InfoCard title="Conversations" icon={Database}>
            {config && (
              <div className="space-y-4">
                <div className="flex items-center justify-between">
                  <span className="text-sm text-muted-foreground">Status:</span>
                  <StatusBadge status={isConversationsEnabled() ? 'active' : 'disabled'} />
                </div>
                {isConversationsEnabled() ? (
                  <>
                    <InfoRow label="Store Type" value={getConversationStore()} />
                    <InfoRow label="Driver" value={getConversationDriver()} />
                    <InfoRow label="TTL" value={getConversationTTL()} />
                    {getMaxConnections() && (
                      <InfoRow label="Max Connections" value={getMaxConnections()} />
                    )}
                  </>
                ) : (
                  <p className="text-xs text-muted-foreground">Conversation storage is currently disabled</p>
                )}
              </div>
            )}
          </InfoCard>

          {/* Rate Limiting */}
          <InfoCard title="Rate Limiting" icon={Settings}>
            {config && (
              <div className="space-y-4">
                <div className="flex items-center justify-between">
                  <span className="text-sm text-muted-foreground">Status:</span>
                  <StatusBadge status={config.rate_limit?.enabled ? 'active' : 'disabled'} />
                </div>
                {config.rate_limit?.enabled ? (
                  <>
                    <InfoRow
                      label="Requests/sec"
                      value={config.rate_limit.requests_per_second ? String(config.rate_limit.requests_per_second) : 'N/A'}
                    />
                    <InfoRow label="Burst" value={config.rate_limit.burst ? String(config.rate_limit.burst) : 'N/A'} />
                    {config.rate_limit.max_concurrent_requests && (
                      <InfoRow label="Max Concurrent" value={String(config.rate_limit.max_concurrent_requests)} />
                    )}
                    {config.rate_limit.daily_token_quota && (
                      <InfoRow label="Daily Token Quota" value={config.rate_limit.daily_token_quota.toLocaleString()} />
                    )}
                  </>
                ) : (
                  <p className="text-xs text-muted-foreground">Rate limiting is currently disabled</p>
                )}
              </div>
            )}
          </InfoCard>
        </div>

        {/* Observability */}
        {config && (
          <div className="rounded-xl border border-border bg-card p-6">
            <div className="mb-6 flex items-center gap-2">
              <BarChart3 className="h-5 w-5 text-muted-foreground" />
              <h2 className="text-lg font-medium tracking-tight">Observability</h2>
            </div>
            <div className="grid gap-6 md:grid-cols-2">
              <div className="space-y-4">
                <div className="flex items-center justify-between border-b border-border pb-2">
                  <span className="text-sm text-muted-foreground">Metrics</span>
                  <StatusBadge status={config.observability?.metrics?.Enabled ? 'active' : 'disabled'} />
                </div>
                {config.observability?.metrics?.Enabled ? (
                  <InfoRow label="Endpoint" value={config.observability.metrics.Path || '/metrics'} mono />
                ) : (
                  <p className="text-xs text-muted-foreground">Metrics collection is disabled</p>
                )}
              </div>
              <div className="space-y-4">
                <div className="flex items-center justify-between border-b border-border pb-2">
                  <span className="text-sm text-muted-foreground">Tracing</span>
                  <StatusBadge status={config.observability?.tracing?.enabled ? 'active' : 'disabled'} />
                </div>
                {config.observability?.tracing?.enabled ? (
                  <>
                    <InfoRow label="Service Name" value={config.observability.tracing.service_name || 'N/A'} />
                    <InfoRow
                      label="Sample Rate"
                      value={
                        config.observability.tracing.sampler?.Rate
                          ? `${(config.observability.tracing.sampler.Rate * 100).toFixed(0)}%`
                          : 'N/A'
                      }
                    />
                  </>
                ) : (
                  <p className="text-xs text-muted-foreground">Distributed tracing is disabled</p>
                )}
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

// Helper Components
function InfoCard({ title, icon: Icon, children }: { title: string; icon: any; children: React.ReactNode }) {
  return (
    <div className="rounded-xl border border-border bg-card p-6">
      <div className="mb-4 flex items-center gap-2">
        <Icon className="h-5 w-5 text-muted-foreground" />
        <h2 className="text-lg font-medium tracking-tight">{title}</h2>
      </div>
      {children}
    </div>
  )
}

function InfoRow({ label, value, mono = false }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="flex items-center justify-between">
      <span className="text-sm text-muted-foreground">{label}:</span>
      <span className={`text-sm ${mono ? 'font-mono' : ''}`}>{value}</span>
    </div>
  )
}

function StatusBadge({ status }: { status: string }) {
  const getStatusColor = () => {
    if (status === 'healthy' || status === 'active') return 'bg-green-500/20 text-green-400 border-green-500/30'
    if (status === 'disabled') return 'bg-muted text-muted-foreground border-border'
    return 'bg-yellow-500/20 text-yellow-400 border-yellow-500/30'
  }

  return (
    <span className={`inline-flex items-center rounded-md border px-2 py-1 text-xs font-medium capitalize ${getStatusColor()}`}>
      {status}
    </span>
  )
}

function HealthItem({ label, status, description }: { label: string; status: string; description: string }) {
  return (
    <div className="flex items-center gap-3">
      <StatusBadge status={status} />
      <div className="flex-1">
        <div className="text-sm capitalize">{label}</div>
        <div className="text-xs text-muted-foreground">{description}</div>
      </div>
    </div>
  )
}

function ProviderCard({
  name,
  status,
  type,
  endpoint,
  apiKey,
  modelsCount,
  models,
}: {
  name: string
  status: string
  type: string
  endpoint?: string
  apiKey: string
  modelsCount: number
  models: string[]
}) {
  const [expanded, setExpanded] = useState(false)

  return (
    <div className="rounded-lg border border-border bg-secondary/50 p-4">
      <div className="mb-3 flex items-start justify-between">
        <div className="flex-1">
          <div className="mb-1 flex items-center gap-2">
            <h3 className="font-medium">{name}</h3>
            <StatusBadge status={status} />
          </div>
          <div className="space-y-1 text-sm text-muted-foreground">
            <div>Type: {type}</div>
            {endpoint && <div>Endpoint: {endpoint}</div>}
            <div>API Key: {apiKey.substring(0, 8)}...</div>
            <div>{modelsCount} model(s)</div>
          </div>
        </div>
        {modelsCount > 0 && (
          <button
            onClick={() => setExpanded(!expanded)}
            className="text-sm text-primary hover:underline"
          >
            {expanded ? 'Hide' : 'Show'} Models
          </button>
        )}
      </div>
      {expanded && models.length > 0 && (
        <div className="mt-3 border-t border-border pt-3">
          <div className="flex flex-wrap gap-2">
            {models.map(model => (
              <span key={model} className="rounded-md bg-muted px-2 py-1 text-xs font-mono">
                {model}
              </span>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
