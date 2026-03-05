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
  name: string
  provider: string
  provider_model_id?: string
}

export interface ConfigResponse {
  server: {
    address: string
    max_request_body_size: number
  }
  providers: Record<string, SanitizedProvider>
  models: ModelEntry[]
  auth: {
    enabled: boolean
    issuer: string
    audience: string
  }
  conversations: {
    store: string
    ttl: string
    dsn: string
    driver: string
  }
  logging: {
    format: string
    level: string
  }
  rate_limit: {
    enabled: boolean
    requests_per_second: number
    burst: number
  }
  observability: any
}

export interface ProviderInfo {
  name: string
  type: string
  models: string[]
  status: string
}
