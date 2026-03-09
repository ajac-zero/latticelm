<template>
  <div class="dashboard">
    <!-- Main Content -->
    <main class="main-content">
      <div class="container">
        <div v-if="loading" class="loading">Loading...</div>
        <div v-else-if="error" class="error">{{ error }}</div>
        <div v-else>
          <!-- Top Grid -->
          <div class="top-grid">
            <!-- Server Configuration -->
            <InfoCard title="Server Configuration" :icon="Server">
              <div class="info-list" v-if="config">
                <InfoRow label="Address" :value="getServerAddress()" :mono="true" />
                <InfoRow label="Max Request Size" :value="getMaxRequestSize()" />
                <InfoRow label="Platform" :value="systemInfo?.platform || 'darwin/arm64'" />
                <InfoRow label="Go Version" :value="systemInfo?.go_version || 'N/A'" />
                <InfoRow label="Uptime" :value="systemInfo?.uptime || 'N/A'" />
              </div>
            </InfoCard>

            <!-- Health Status -->
            <InfoCard title="Health Status" :icon="Activity">
              <div v-if="health">
                <div class="health-overall">
                  <span class="health-label">Overall Status:</span>
                  <StatusBadge :status="health.status" />
                </div>

                <div class="health-divider"></div>

                <div class="health-checks">
                  <HealthItem
                    label="Conversation Store"
                    :status="health.checks.conversation_store?.status || 'unknown'"
                    :description="health.checks.conversation_store?.message || 'PostgreSQL connected'"
                  />
                  <HealthItem
                    label="Providers"
                    :status="health.checks.providers?.status || 'healthy'"
                    :description="`${config ? Object.keys(config.providers).length : 0} provider(s) configured`"
                  />
                  <HealthItem
                    label="Server"
                    :status="health.checks.server?.status || 'healthy'"
                    :description="health.checks.server?.message || 'Server is running'"
                  />
                </div>
              </div>
            </InfoCard>
          </div>

          <!-- Providers Section -->
          <div class="providers-section">
            <div class="section-header">
              <Database :size="20" class="section-icon" />
              <h2 class="section-title">Providers</h2>
            </div>

            <div v-if="config && config.providers" class="providers-list">
              <ProviderCard
                v-for="(providerConfig, name) in config.providers"
                :key="name"
                :name="String(name)"
                :status="getProviderStatus(String(name))"
                :type="providerConfig.type"
                :endpoint="providerConfig.endpoint"
                :api-key="providerConfig.api_key"
                :models-count="getProviderModels(String(name)).length"
                :models="getProviderModels(String(name))"
              />
            </div>
            <div v-else class="empty-state">No providers configured</div>
          </div>

          <!-- Bottom Grid -->
          <div class="bottom-grid">
            <!-- Authentication -->
            <InfoCard title="Authentication" :icon="Lock">
              <div class="info-list" v-if="config">
                <div class="status-row">
                  <span class="info-label">Status:</span>
                  <StatusBadge :status="config.auth?.enabled ? 'active' : 'disabled'" />
                </div>
                <template v-if="config.auth?.enabled">
                  <InfoRow label="Issuer" :value="config.auth.issuer || 'Not configured'" />
                  <InfoRow label="Audience" :value="config.auth.audience || 'Not configured'" />
                </template>
                <p v-else class="info-text">Authentication is currently disabled</p>
              </div>
            </InfoCard>

            <!-- Conversations -->
            <InfoCard title="Conversations" :icon="Database">
              <div class="info-list" v-if="config">
                <div class="status-row">
                  <span class="info-label">Status:</span>
                  <StatusBadge :status="isConversationsEnabled() ? 'active' : 'disabled'" />
                </div>
                <template v-if="isConversationsEnabled()">
                  <InfoRow label="Store Type" :value="getConversationStore()" />
                  <InfoRow label="Driver" :value="getConversationDriver()" />
                  <InfoRow label="TTL" :value="getConversationTTL()" />
                  <InfoRow v-if="getMaxConnections()" label="Max Connections" :value="getMaxConnections()" />
                </template>
                <p v-else class="info-text">Conversation storage is currently disabled</p>
              </div>
            </InfoCard>

            <!-- Rate Limiting -->
            <InfoCard title="Rate Limiting" :icon="Settings">
              <div class="info-list" v-if="config">
                <div class="status-row">
                  <span class="info-label">Status:</span>
                  <StatusBadge :status="config.rate_limit?.enabled ? 'active' : 'disabled'" />
                </div>
                <template v-if="config.rate_limit?.enabled">
                  <InfoRow label="Requests/sec" :value="config.rate_limit.requests_per_second ? String(config.rate_limit.requests_per_second) : 'N/A'" />
                  <InfoRow label="Burst" :value="config.rate_limit.burst ? String(config.rate_limit.burst) : 'N/A'" />
                  <InfoRow v-if="config.rate_limit.max_concurrent_requests" label="Max Concurrent" :value="String(config.rate_limit.max_concurrent_requests)" />
                  <InfoRow v-if="config.rate_limit.daily_token_quota" label="Daily Token Quota" :value="config.rate_limit.daily_token_quota.toLocaleString()" />
                </template>
                <p v-else class="info-text">Rate limiting is currently disabled</p>
              </div>
            </InfoCard>
          </div>

          <!-- Observability -->
          <div v-if="config" class="observability-section">
            <div class="section-header">
              <BarChart3 :size="20" class="section-icon" />
              <h2 class="section-title">Observability</h2>
            </div>
            <div class="observability-grid">
              <div class="observability-card">
                <div class="observability-header">
                  <span class="info-label">Metrics</span>
                  <StatusBadge :status="config.observability?.metrics?.Enabled ? 'active' : 'disabled'" />
                </div>
                <InfoRow v-if="config.observability?.metrics?.Enabled" label="Endpoint" :value="config.observability.metrics.Path || '/metrics'" :mono="true" />
                <p v-else class="info-text">Metrics collection is disabled</p>
              </div>
              <div class="observability-card">
                <div class="observability-header">
                  <span class="info-label">Tracing</span>
                  <StatusBadge :status="config.observability?.tracing?.enabled ? 'active' : 'disabled'" />
                </div>
                <template v-if="config.observability?.tracing?.enabled">
                  <InfoRow label="Service Name" :value="config.observability.tracing.service_name || 'N/A'" />
                  <InfoRow label="Sample Rate" :value="config.observability.tracing.sampler?.Rate ? `${(config.observability.tracing.sampler.Rate * 100).toFixed(0)}%` : 'N/A'" />
                </template>
                <p v-else class="info-text">Distributed tracing is disabled</p>
              </div>
            </div>
          </div>
        </div>
      </div>
    </main>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { Activity, Server, Database, Lock, Settings, BarChart3 } from 'lucide-vue-next'
import { systemAPI } from '../api/system'
import { configAPI } from '../api/config'
import { providersAPI } from '../api/providers'
import type { SystemInfo, HealthCheckResponse, ConfigResponse, ProviderInfo } from '../types/api'
import StatusBadge from '../components/StatusBadge.vue'
import InfoCard from '../components/InfoCard.vue'
import ProviderCard from '../components/ProviderCard.vue'
import InfoRow from '../components/InfoRow.vue'
import HealthItem from '../components/HealthItem.vue'

const loading = ref(true)
const error = ref<string | null>(null)
const systemInfo = ref<SystemInfo | null>(null)
const health = ref<HealthCheckResponse | null>(null)
const config = ref<ConfigResponse | null>(null)
const providers = ref<ProviderInfo[] | null>(null)

let refreshInterval: number | null = null

function getProviderStatus(providerName: string): string {
  const provider = providers.value?.find(p => p.name === providerName)
  return provider?.status || 'active'
}

function getProviderModels(providerName: string): string[] {
  if (!config.value) return []
  return config.value.models
    .filter(m => {
      const modelProvider = (m as any).Provider || m.provider
      return modelProvider === providerName
    })
    .map(m => (m as any).Name || m.name)
}

function getServerAddress(): string {
  if (!config.value) return 'N/A'
  // Handle both lowercase and capitalized versions
  const addr = (config.value.server as any).Address || config.value.server.address
  return addr || ':8080'
}

function getMaxRequestSize(): string {
  if (!config.value) return 'N/A'
  // Handle both snake_case and camelCase versions
  const size = (config.value.server as any).MaxRequestBodySize || config.value.server.max_request_body_size
  if (!size || isNaN(size)) return '10 MB'
  return `${(size / 1024 / 1024).toFixed(0)} MB`
}

function isConversationsEnabled(): boolean {
  if (!config.value) return false
  const convConfig = config.value.conversations as any
  return convConfig.Enabled === true || !!(convConfig.Store || convConfig.store)
}

function getConversationStore(): string {
  if (!config.value) return 'N/A'
  const convConfig = config.value.conversations as any
  return convConfig.Store || convConfig.store || 'N/A'
}

function getConversationDriver(): string {
  if (!config.value) return 'N/A'
  const convConfig = config.value.conversations as any
  return convConfig.Driver || convConfig.driver || 'N/A'
}

function getConversationTTL(): string {
  if (!config.value) return 'N/A'
  const convConfig = config.value.conversations as any
  return convConfig.TTL || convConfig.ttl || 'N/A'
}

function getMaxConnections(): string {
  if (!config.value) return ''
  const convConfig = config.value.conversations as any
  const maxConns = convConfig.MaxOpenConns || convConfig.max_open_conns
  return maxConns ? String(maxConns) : ''
}

async function loadData() {
  try {
    loading.value = true
    error.value = null

    const [info, healthData, configData, providersData] = await Promise.all([
      systemAPI.getInfo(),
      systemAPI.getHealth(),
      configAPI.getConfig(),
      providersAPI.getProviders(),
    ])

    systemInfo.value = info
    health.value = healthData
    config.value = configData
    providers.value = providersData
  } catch (err: any) {
    error.value = err.message || 'Failed to load data'
    console.error('Error loading data:', err)
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  loadData()
  // Auto-refresh every 30 seconds
  refreshInterval = window.setInterval(loadData, 30000)
})

onUnmounted(() => {
  if (refreshInterval) {
    clearInterval(refreshInterval)
  }
})
</script>

<style scoped>
.dashboard {
  min-height: 100vh;
  background-color: var(--background);
}

/* Main Content */
.main-content {
  padding: 2rem 0;
}

.container {
  max-width: 1400px;
  margin: 0 auto;
  padding: 0 1.5rem;
}

.loading,
.error {
  text-align: center;
  padding: 3rem;
  font-size: 1.2rem;
}

.error {
  color: rgb(248, 113, 113);
}

/* Top Grid */
.top-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(400px, 1fr));
  gap: 1.5rem;
  margin-bottom: 1.5rem;
}

/* Info Rows */
.info-list {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.info-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.5rem 0;
}

.info-label {
  font-size: 0.875rem;
  color: rgb(161, 161, 170);
}

.info-value {
  font-size: 0.875rem;
  color: var(--foreground);
}

.info-value.mono {
  font-family: ui-monospace, SFMono-Regular, 'SF Mono', Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace;
}

/* Health Status */
.health-overall {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.5rem 0;
}

.health-label {
  font-size: 0.875rem;
  color: rgb(161, 161, 170);
}

.health-divider {
  height: 1px;
  background-color: var(--border);
  margin: 1rem 0;
}

.health-checks {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.health-item {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.health-item-content {
  flex: 1;
}

.health-item-label {
  font-size: 0.875rem;
  color: var(--foreground);
  text-transform: capitalize;
}

.health-item-description {
  font-size: 0.75rem;
  color: rgb(161, 161, 170);
}

/* Providers Section */
.providers-section {
  background-color: var(--card);
  border: 1px solid var(--border);
  border-radius: 0.75rem;
  padding: 1.5rem;
  margin-bottom: 1.5rem;
}

.section-header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-bottom: 1.5rem;
}

.section-header.clickable {
  cursor: pointer;
  user-select: none;
  justify-content: space-between;
}

.section-header-left {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.section-icon {
  color: rgb(161, 161, 170);
}

.section-title {
  font-size: 1.125rem;
  font-weight: 500;
  letter-spacing: -0.01em;
  color: var(--foreground);
  margin: 0;
}

.providers-list {
  display: grid;
  gap: 1rem;
}

.empty-state {
  text-align: center;
  padding: 2rem;
  color: rgb(161, 161, 170);
}

/* Bottom Grid */
.bottom-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
  gap: 1.5rem;
  margin-bottom: 1.5rem;
}

.status-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.5rem 0;
}

.info-text {
  font-size: 0.75rem;
  color: rgb(161, 161, 170);
  margin: 0.5rem 0;
}

/* Observability Section */
.observability-section {
  background-color: var(--card);
  border: 1px solid var(--border);
  border-radius: 0.75rem;
  padding: 1.5rem;
}

.observability-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: 1.5rem;
}

.observability-card {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.observability-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding-bottom: 0.5rem;
}
</style>
