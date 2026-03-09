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
    audience?: string
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
}

export interface ProviderInfo {
  name: string
  type: string
  models: string[]
  status: string
}

export interface User {
  email: string
  is_admin: boolean
}
