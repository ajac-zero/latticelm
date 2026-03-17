import { createFileRoute } from '@tanstack/react-router'
import { useState, useMemo } from 'react'
import { Activity, Server, Database, Lock, Settings, BarChart3, Plus, Trash2, Pencil, Cpu } from 'lucide-react'
import { Skeleton } from '#/components/ui/skeleton'
import {
  useSystemInfo,
  useHealth,
  useConfig,
  useProviders,
  useConfigProviders,
  useCreateProvider,
  useUpdateProvider,
  useDeleteProvider,
  useConfigModels,
  useCreateModel,
  useUpdateModel,
  useDeleteModel,
} from '../lib/api/hooks'
import { requireAdmin } from '../lib/auth'
import { Button } from '#/components/ui/button'
import { Input } from '#/components/ui/input'
import { Label } from '#/components/ui/label'
import { Badge } from '#/components/ui/badge'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '#/components/ui/tabs'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '#/components/ui/table'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '#/components/ui/dialog'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '#/components/ui/alert-dialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '#/components/ui/select'
import type { ProviderInfo, ProviderEntry, ConfigModelEntry } from '../lib/api/types'

export const Route = createFileRoute('/system')({
  beforeLoad: requireAdmin,
  component: DashboardPage,
})

function DashboardPage() {
  return (
    <div className="h-full overflow-auto bg-background py-8">
      <div className="container mx-auto max-w-[1400px] px-6">
        <div className="mb-6 flex items-center gap-2">
          <Settings className="h-6 w-6 text-muted-foreground" />
          <h1 className="text-2xl font-medium tracking-tight">System</h1>
        </div>
        <Tabs defaultValue="status">
          <TabsList className="mb-6">
            <TabsTrigger value="status" className="gap-2">
              <Activity className="h-4 w-4" />
              Status
            </TabsTrigger>
            <TabsTrigger value="configuration" className="gap-2">
              <Database className="h-4 w-4" />
              Configuration
            </TabsTrigger>
          </TabsList>
          <TabsContent value="status">
            <StatusTab />
          </TabsContent>
          <TabsContent value="configuration">
            <ConfigurationTab />
          </TabsContent>
        </Tabs>
      </div>
    </div>
  )
}

// ─── Status Tab ──────────────────────────────────────────────────────────────

function StatusTab() {
  const { data: systemInfo, isLoading: systemLoading } = useSystemInfo()
  const { data: health, isLoading: healthLoading } = useHealth()
  const { data: config, isLoading: configLoading } = useConfig()
  const { data: providers, isLoading: providersLoading } = useProviders()

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
    <div className="space-y-6">
      {/* Top Grid */}
      <div className="grid gap-6 md:grid-cols-2">
        <InfoCard title="Server Configuration" icon={Server} isLoading={systemLoading || configLoading}>
          <div className="space-y-4">
            <InfoRow label="Address" value={getServerAddress()} mono />
            <InfoRow label="Max Request Size" value={getMaxRequestSize()} />
            <InfoRow label="Platform" value={systemInfo?.platform || 'darwin/arm64'} />
            <InfoRow label="Go Version" value={systemInfo?.go_version || 'N/A'} />
            <InfoRow label="Uptime" value={systemInfo?.uptime || 'N/A'} />
          </div>
        </InfoCard>

        <InfoCard title="Health Status" icon={Activity} isLoading={healthLoading}>
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
      <div className="rounded-xl border border-border bg-card p-6">
        <div className="mb-6 flex items-center gap-2">
          <Database className="h-5 w-5 text-muted-foreground" />
          <h2 className="text-lg font-medium tracking-tight">Providers</h2>
        </div>
        {configLoading || providersLoading ? (
          <div className="space-y-3">
            <Skeleton className="h-20 w-full rounded-lg" />
            <Skeleton className="h-20 w-full rounded-lg" />
          </div>
        ) : config && config.providers && Object.keys(config.providers).length > 0 ? (
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
      <div className="grid gap-6 md:grid-cols-3">
        <InfoCard title="Authentication" icon={Lock} isLoading={configLoading}>
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

        <InfoCard title="Conversations" icon={Database} isLoading={configLoading}>
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

        <InfoCard title="Rate Limiting" icon={Settings} isLoading={configLoading}>
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
      <div className="rounded-xl border border-border bg-card p-6">
        <div className="mb-6 flex items-center gap-2">
          <BarChart3 className="h-5 w-5 text-muted-foreground" />
          <h2 className="text-lg font-medium tracking-tight">Observability</h2>
        </div>
        {configLoading ? (
          <div className="grid gap-6 md:grid-cols-2">
            <div className="space-y-3">
              <Skeleton className="h-4 w-full" />
              <Skeleton className="h-4 w-3/4" />
              <Skeleton className="h-4 w-1/2" />
            </div>
            <div className="space-y-3">
              <Skeleton className="h-4 w-full" />
              <Skeleton className="h-4 w-3/4" />
              <Skeleton className="h-4 w-1/2" />
            </div>
          </div>
        ) : config ? (
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
        ) : null}
      </div>
    </div>
  )
}

// ─── Configuration Tab ───────────────────────────────────────────────────────

// Provider type definitions — mirrors backend config.go validation logic.
type ProviderFieldConfig = {
  label: string
  placeholder: string
  required: boolean
  type?: 'password' | 'text'
}

type ProviderTypeConfig = {
  label: string
  fields: Partial<Record<keyof ProviderEntry, ProviderFieldConfig>>
}

const PROVIDER_TYPES: Record<string, ProviderTypeConfig> = {
  openai: {
    label: 'OpenAI',
    fields: {
      api_key: { label: 'API Key', placeholder: 'sk-...', required: true, type: 'password' },
      endpoint: { label: 'Endpoint', placeholder: 'https://api.openai.com/v1', required: false },
    },
  },
  anthropic: {
    label: 'Anthropic',
    fields: {
      api_key: { label: 'API Key', placeholder: 'sk-ant-...', required: true, type: 'password' },
      endpoint: { label: 'Endpoint', placeholder: 'https://api.anthropic.com', required: false },
    },
  },
  google: {
    label: 'Google (Gemini)',
    fields: {
      api_key: { label: 'API Key', placeholder: 'AIza...', required: true, type: 'password' },
      endpoint: { label: 'Endpoint', placeholder: 'https://generativelanguage.googleapis.com', required: false },
    },
  },
  azureopenai: {
    label: 'Azure OpenAI',
    fields: {
      api_key: { label: 'API Key', placeholder: '...', required: true, type: 'password' },
      endpoint: { label: 'Endpoint', placeholder: 'https://<resource>.openai.azure.com', required: true },
      api_version: { label: 'API Version', placeholder: '2024-02-01', required: false },
    },
  },
  azureanthropic: {
    label: 'Azure Anthropic',
    fields: {
      api_key: { label: 'API Key', placeholder: '...', required: true, type: 'password' },
      endpoint: { label: 'Endpoint', placeholder: 'https://<resource>.services.ai.azure.com', required: true },
    },
  },
  vertexai: {
    label: 'Vertex AI',
    fields: {
      project: { label: 'GCP Project', placeholder: 'my-gcp-project', required: true },
      location: { label: 'Location', placeholder: 'us-central1', required: true },
    },
  },
}

function ConfigurationTab() {
  return (
    <div className="space-y-8">
      <ProvidersSection />
      <ModelsSection />
    </div>
  )
}

function ProvidersSection() {
  const { data: providers, isLoading } = useConfigProviders()
  const createProvider = useCreateProvider()
  const updateProvider = useUpdateProvider()
  const deleteProvider = useDeleteProvider()

  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingProvider, setEditingProvider] = useState<ProviderInfo | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null)

  const emptyForm = { name: '', type: '', api_key: '', endpoint: '', api_version: '', project: '', location: '' }
  const [form, setForm] = useState<{ name: string } & ProviderEntry>(emptyForm)

  const typeConfig = form.type ? PROVIDER_TYPES[form.type] : null

  const isFormValid = useMemo(() => {
    if (!form.type || (!editingProvider && !form.name)) return false
    if (!typeConfig) return false
    return Object.entries(typeConfig.fields).every(([key, cfg]) => {
      if (!cfg.required) return true
      if (editingProvider && cfg.type === 'password') return true
      return !!form[key as keyof typeof form]
    })
  }, [form, typeConfig, editingProvider])

  const openCreate = () => {
    setEditingProvider(null)
    setForm(emptyForm)
    setDialogOpen(true)
  }

  const openEdit = (provider: ProviderInfo) => {
    setEditingProvider(provider)
    setForm({ ...emptyForm, name: provider.name, type: provider.type })
    setDialogOpen(true)
  }

  const handleTypeChange = (type: string) => {
    setForm({ ...emptyForm, name: form.name, type })
  }

  const handleSubmit = async () => {
    try {
      const payload: ProviderEntry = {
        type: form.type,
        api_key: form.api_key || undefined,
        endpoint: form.endpoint || undefined,
        api_version: form.api_version || undefined,
        project: form.project || undefined,
        location: form.location || undefined,
      }
      if (editingProvider) {
        await updateProvider.mutateAsync({ name: editingProvider.name, data: payload })
      } else {
        await createProvider.mutateAsync({ name: form.name, ...payload })
      }
      setDialogOpen(false)
    } catch (error) {
      alert(error instanceof Error ? error.message : 'Operation failed')
    }
  }

  const handleDelete = async () => {
    if (!deleteTarget) return
    try {
      await deleteProvider.mutateAsync(deleteTarget)
      setDeleteTarget(null)
    } catch (error) {
      alert(error instanceof Error ? error.message : 'Failed to delete provider')
    }
  }

  const isPending = createProvider.isPending || updateProvider.isPending

  return (
    <div>
      <div className="mb-4 flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Database className="h-5 w-5 text-muted-foreground" />
          <h2 className="text-lg font-medium tracking-tight">Providers</h2>
          <span className="text-sm text-muted-foreground">
            ({providers?.length ?? 0})
          </span>
        </div>
        <Button size="sm" onClick={openCreate}>
          <Plus className="mr-2 h-4 w-4" />
          Add Provider
        </Button>
      </div>

      <div className="rounded-xl border border-border bg-card">
        {isLoading ? (
          <div className="flex items-center justify-center py-16 text-muted-foreground">Loading...</div>
        ) : !providers || providers.length === 0 ? (
          <div className="flex items-center justify-center py-16 text-muted-foreground">No providers configured</div>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Type</TableHead>
                <TableHead>Models</TableHead>
                <TableHead>Status</TableHead>
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {providers.map((provider) => (
                <TableRow key={provider.name}>
                  <TableCell className="font-mono font-medium">{provider.name}</TableCell>
                  <TableCell>{PROVIDER_TYPES[provider.type]?.label ?? provider.type}</TableCell>
                  <TableCell>
                    <span className="text-sm text-muted-foreground">
                      {provider.models?.length ?? 0} model{(provider.models?.length ?? 0) !== 1 ? 's' : ''}
                    </span>
                  </TableCell>
                  <TableCell>
                    <Badge variant={provider.status === 'active' ? 'default' : 'secondary'} className="capitalize">
                      {provider.status}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center justify-end gap-2">
                      <Button variant="ghost" size="sm" onClick={() => openEdit(provider)}>
                        <Pencil className="h-4 w-4" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="text-destructive hover:text-destructive"
                        onClick={() => setDeleteTarget(provider.name)}
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </div>

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="sm:max-w-[460px]">
          <DialogHeader>
            <DialogTitle>{editingProvider ? `Edit Provider: ${editingProvider.name}` : 'Add Provider'}</DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-2">
            {!editingProvider && (
              <div className="space-y-1.5">
                <Label htmlFor="prov-name">Name</Label>
                <Input
                  id="prov-name"
                  placeholder="my-openai"
                  value={form.name}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                />
              </div>
            )}
            <div className="space-y-1.5">
              <Label htmlFor="prov-type">Type</Label>
              <Select value={form.type} onValueChange={handleTypeChange} disabled={!!editingProvider}>
                <SelectTrigger id="prov-type">
                  <SelectValue placeholder="Select provider type" />
                </SelectTrigger>
                <SelectContent>
                  {Object.entries(PROVIDER_TYPES).map(([value, cfg]) => (
                    <SelectItem key={value} value={value}>{cfg.label}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            {typeConfig && Object.entries(typeConfig.fields).map(([key, cfg]) => (
              <div key={key} className="space-y-1.5">
                <Label htmlFor={`prov-${key}`}>
                  {cfg.label}
                  {!cfg.required && <span className="ml-1 text-xs text-muted-foreground">(optional)</span>}
                  {cfg.required && editingProvider && cfg.type === 'password' && (
                    <span className="ml-1 text-xs text-muted-foreground">(leave blank to keep current)</span>
                  )}
                </Label>
                <Input
                  id={`prov-${key}`}
                  type={cfg.type === 'password' ? 'password' : 'text'}
                  placeholder={editingProvider && cfg.type === 'password' ? '••••••••' : cfg.placeholder}
                  value={(form[key as keyof typeof form] as string) ?? ''}
                  onChange={(e) => setForm({ ...form, [key]: e.target.value })}
                />
              </div>
            ))}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDialogOpen(false)}>Cancel</Button>
            <Button onClick={handleSubmit} disabled={isPending || !isFormValid}>
              {isPending ? 'Saving...' : editingProvider ? 'Save Changes' : 'Add Provider'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <AlertDialog open={!!deleteTarget} onOpenChange={(open) => !open && setDeleteTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete Provider</AlertDialogTitle>
            <AlertDialogDescription>
              Are you sure you want to delete provider <strong>{deleteTarget}</strong>? This cannot be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleDelete}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

function ModelsSection() {
  const { data: models, isLoading } = useConfigModels()
  const { data: providers } = useConfigProviders()
  const createModel = useCreateModel()
  const updateModel = useUpdateModel()
  const deleteModel = useDeleteModel()

  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingModel, setEditingModel] = useState<ConfigModelEntry | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null)

  const [form, setForm] = useState<ConfigModelEntry>({ name: '', provider: '', provider_model_id: '' })

  const normalizedModels: ConfigModelEntry[] = (models ?? []).map((m: any) => ({
    name: m.Name || m.name || '',
    provider: m.Provider || m.provider || '',
    provider_model_id: m.ProviderModelID || m.provider_model_id || '',
  }))

  const providerNames = providers?.map((p) => p.name) ?? []

  const openCreate = () => {
    setEditingModel(null)
    setForm({ name: '', provider: '', provider_model_id: '' })
    setDialogOpen(true)
  }

  const openEdit = (model: ConfigModelEntry) => {
    setEditingModel(model)
    setForm({ ...model })
    setDialogOpen(true)
  }

  const handleSubmit = async () => {
    try {
      if (editingModel) {
        await updateModel.mutateAsync({
          name: editingModel.name,
          data: { provider: form.provider, provider_model_id: form.provider_model_id || undefined },
        })
      } else {
        await createModel.mutateAsync({
          name: form.name,
          provider: form.provider,
          provider_model_id: form.provider_model_id || undefined,
        })
      }
      setDialogOpen(false)
    } catch (error) {
      alert(error instanceof Error ? error.message : 'Operation failed')
    }
  }

  const handleDelete = async () => {
    if (!deleteTarget) return
    try {
      await deleteModel.mutateAsync(deleteTarget)
      setDeleteTarget(null)
    } catch (error) {
      alert(error instanceof Error ? error.message : 'Failed to delete model')
    }
  }

  const isPending = createModel.isPending || updateModel.isPending

  return (
    <div>
      <div className="mb-4 flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Cpu className="h-5 w-5 text-muted-foreground" />
          <h2 className="text-lg font-medium tracking-tight">Models</h2>
          <span className="text-sm text-muted-foreground">
            ({normalizedModels.length})
          </span>
        </div>
        <Button size="sm" onClick={openCreate}>
          <Plus className="mr-2 h-4 w-4" />
          Add Model
        </Button>
      </div>

      <div className="rounded-xl border border-border bg-card">
        {isLoading ? (
          <div className="flex items-center justify-center py-16 text-muted-foreground">Loading...</div>
        ) : normalizedModels.length === 0 ? (
          <div className="flex items-center justify-center py-16 text-muted-foreground">No models configured</div>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Provider</TableHead>
                <TableHead>Provider Model ID</TableHead>
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {normalizedModels.map((model) => (
                <TableRow key={model.name}>
                  <TableCell className="font-mono font-medium">{model.name}</TableCell>
                  <TableCell>
                    <Badge variant="secondary">{model.provider}</Badge>
                  </TableCell>
                  <TableCell className="font-mono text-sm text-muted-foreground">
                    {model.provider_model_id || '—'}
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center justify-end gap-2">
                      <Button variant="ghost" size="sm" onClick={() => openEdit(model)}>
                        <Pencil className="h-4 w-4" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="text-destructive hover:text-destructive"
                        onClick={() => setDeleteTarget(model.name)}
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </div>

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="sm:max-w-[440px]">
          <DialogHeader>
            <DialogTitle>{editingModel ? `Edit Model: ${editingModel.name}` : 'Add Model'}</DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-2">
            {!editingModel && (
              <div className="space-y-1.5">
                <Label htmlFor="model-name">Name</Label>
                <Input
                  id="model-name"
                  placeholder="gpt-4o"
                  value={form.name}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                />
              </div>
            )}
            <div className="space-y-1.5">
              <Label htmlFor="model-provider">Provider</Label>
              {providerNames.length > 0 ? (
                <Select value={form.provider} onValueChange={(v) => setForm({ ...form, provider: v })}>
                  <SelectTrigger id="model-provider">
                    <SelectValue placeholder="Select provider" />
                  </SelectTrigger>
                  <SelectContent>
                    {providerNames.map((name) => (
                      <SelectItem key={name} value={name}>{name}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              ) : (
                <Input
                  id="model-provider"
                  placeholder="openai"
                  value={form.provider}
                  onChange={(e) => setForm({ ...form, provider: e.target.value })}
                />
              )}
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="model-provider-id">
                Provider Model ID
                <span className="ml-1 text-xs text-muted-foreground">(optional)</span>
              </Label>
              <Input
                id="model-provider-id"
                placeholder="gpt-4o-2024-11-20"
                value={form.provider_model_id ?? ''}
                onChange={(e) => setForm({ ...form, provider_model_id: e.target.value })}
              />
              <p className="text-xs text-muted-foreground">
                The actual model identifier sent to the provider. Defaults to the name above.
              </p>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDialogOpen(false)}>Cancel</Button>
            <Button onClick={handleSubmit} disabled={isPending || !form.provider || (!editingModel && !form.name)}>
              {isPending ? 'Saving...' : editingModel ? 'Save Changes' : 'Add Model'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <AlertDialog open={!!deleteTarget} onOpenChange={(open) => !open && setDeleteTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete Model</AlertDialogTitle>
            <AlertDialogDescription>
              Are you sure you want to delete model <strong>{deleteTarget}</strong>? This cannot be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleDelete}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

// ─── Shared Helper Components ────────────────────────────────────────────────

function InfoCard({ title, icon: Icon, children, isLoading = false }: { title: string; icon: any; children: React.ReactNode; isLoading?: boolean }) {
  return (
    <div className="rounded-xl border border-border bg-card p-6">
      <div className="mb-4 flex items-center gap-2">
        <Icon className="h-5 w-5 text-muted-foreground" />
        <h2 className="text-lg font-medium tracking-tight">{title}</h2>
      </div>
      {isLoading ? (
        <div className="space-y-3">
          <Skeleton className="h-4 w-3/4" />
          <Skeleton className="h-4 w-1/2" />
          <Skeleton className="h-4 w-2/3" />
          <Skeleton className="h-4 w-1/2" />
        </div>
      ) : children}
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
            <div>Type: {PROVIDER_TYPES[type]?.label ?? type}</div>
            {endpoint && <div>Endpoint: {endpoint}</div>}
            <div>API Key: {apiKey ? apiKey.substring(0, 8) + '...' : 'not set'}</div>
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
