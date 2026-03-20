export interface APIResponse<T = any> {
  success: boolean
  data?: T
  error?: APIError
}

export interface APIError {
  code: string
  message: string
}

export interface SystemInfo {
  version: string
  build_time: string
  git_commit: string
  go_version: string
  platform: string
  uptime: string
}

export interface HealthCheck {
  status: string
  message?: string
}

export interface HealthCheckResponse {
  status: string
  timestamp: string
  checks: Record<string, HealthCheck>
}

export interface SanitizedProvider {
  type: string
  api_key: string
  endpoint?: string
  api_version?: string
  project?: string
  location?: string
}

export interface ModelEntry {
  Name?: string
  name?: string
  Provider?: string
  provider?: string
  ProviderModelID?: string
  provider_model_id?: string
}

export interface ConfigResponse {
  server: {
    Address?: string
    address?: string
    MaxRequestBodySize?: number
    max_request_body_size?: number
  }
  providers: Record<string, SanitizedProvider>
  models: ModelEntry[]
  auth?: {
    enabled: boolean
    issuer?: string
    audiences?: string[]
  }
  conversations: {
    Enabled?: boolean
    StoreByDefault?: boolean
    Store?: string
    store?: string
    TTL?: string
    ttl?: string
    DSN?: string
    dsn?: string
    Driver?: string
    driver?: string
    MaxOpenConns?: number
    max_open_conns?: number
    MaxIdleConns?: number
    max_idle_conns?: number
    ConnMaxLifetime?: string
    conn_max_lifetime?: string
    ConnMaxIdleTime?: string
    conn_max_idle_time?: string
  }
  logging?: {
    format?: string
    level?: string
  }
  rate_limit?: {
    enabled: boolean
    requests_per_second?: number
    burst?: number
    max_concurrent_requests?: number
    daily_token_quota?: number
    max_prompt_tokens?: number
    max_output_tokens?: number
    redis_url?: string
    trusted_proxy_cidrs?: string[]
  }
  observability?: {
    enabled?: boolean
    metrics?: {
      Enabled: boolean
      Path?: string
    }
    tracing?: {
      enabled: boolean
      service_name?: string
      sampler?: {
        Type?: string
        Rate?: number
      }
      exporter?: {
        type?: string
        endpoint?: string
        insecure?: boolean
      }
    }
  }
  usage?: {
    enabled: boolean
  }
}

export interface ProviderInfo {
  name: string
  type: string
  models: string[]
  status: string
}

export interface ProviderEntry {
  type: string
  api_key: string
  endpoint?: string
  api_version?: string
  project?: string
  location?: string
}

export interface ProviderEntryRequest extends ProviderEntry {
  name: string
}

export interface ConfigModelEntry {
  name: string
  provider: string
  provider_model_id?: string
}

export interface User {
  email: string
  name?: string
  is_admin: boolean
}

// User Management Types
export interface UserDetail {
  id: string
  email: string
  name: string
  role: 'admin' | 'user'
  status: 'active' | 'suspended' | 'deleted'
  created_at: string
  updated_at?: string
  oidc_iss?: string
  oidc_sub?: string
}

export interface ListUsersResponse {
  users: UserDetail[]
  total: number
  page: number
  limit: number
}

export interface UpdateUserRequest {
  role?: 'admin' | 'user'
  status?: 'active' | 'suspended' | 'deleted'
}

export interface BulkUpdateUserRequest {
  ids: string[]
  role?: 'admin' | 'user'
  status?: 'active' | 'suspended' | 'deleted'
}

// Conversation Types
export interface ConversationItem {
  id: string
  model: string
  message_count: number
  created_at: string
  updated_at: string
}

export interface ConversationMessage {
  role: string
  content: string
  created_at?: string
}

export interface ConversationDetail {
  id: string
  model: string
  messages: ConversationMessage[]
  created_at: string
  updated_at: string
}

export interface ListConversationsResponse {
  conversations: ConversationItem[]
  total: number
  page: number
  limit: number
}

// Usage Analytics Types
export interface UsageSummaryRow {
  tenant_id: string
  user_sub: string
  provider: string
  model: string
  input_tokens: number
  output_tokens: number
  total_tokens: number
  request_count: number
}

export interface UsageSummaryResponse {
  data: UsageSummaryRow[]
  start: string
  end: string
}

export interface UsageTopRow {
  key: string
  input_tokens: number
  output_tokens: number
  total_tokens: number
  request_count: number
}

export interface UsageTopResponse {
  dimension: string
  data: UsageTopRow[]
  start: string
  end: string
}

export interface UsageTrendRow {
  bucket: string
  key?: string
  input_tokens: number
  output_tokens: number
  total_tokens: number
  request_count: number
}

export interface UsageTrendsResponse {
  granularity: string
  data: UsageTrendRow[]
  start: string
  end: string
}
