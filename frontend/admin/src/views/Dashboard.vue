<template>
  <div class="dashboard">
    <!-- Header -->
    <header class="header">
      <div class="container">
        <div class="header-content">
          <div class="header-left">
            <div class="logo">
              <Zap :size="16" class="logo-icon" />
            </div>
            <h1 class="header-title">LLM Gateway Admin</h1>
          </div>
          <router-link to="/chat" class="playground-button">
            <span>Playground</span>
            <ArrowRight :size="16" class="arrow-icon" />
          </router-link>
        </div>
      </div>
    </header>

    <!-- Main Content -->
    <main class="main-content">
      <div class="container">
        <div v-if="loading" class="loading">Loading...</div>
        <div v-else-if="error" class="error">{{ error }}</div>
        <div v-else>
          <!-- Top Grid -->
          <div class="top-grid">
            <!-- System Information -->
            <InfoCard title="System Information" :icon="Server">
              <div class="info-list" v-if="systemInfo">
                <InfoRow label="Version" :value="systemInfo.version" />
                <InfoRow label="Platform" :value="systemInfo.platform" />
                <InfoRow label="Go Version" :value="systemInfo.go_version" />
                <InfoRow label="Uptime" :value="systemInfo.uptime" />
                <InfoRow label="Build Time" :value="systemInfo.build_time" />
                <InfoRow label="Git Commit" :value="systemInfo.git_commit" :mono="true" />
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
                    v-for="(check, name) in health.checks"
                    :key="name"
                    :label="String(name)"
                    :status="check.status"
                    :description="check.message || ''"
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

            <div v-if="providers && providers.length > 0" class="providers-list">
              <ProviderCard
                v-for="provider in providers"
                :key="provider.name"
                :name="provider.name"
                :status="provider.status"
                :type="provider.type"
                :models-count="provider.models.length"
                :models="provider.models"
              />
            </div>
            <div v-else class="empty-state">No providers configured</div>
          </div>

          <!-- Config Section (collapsed by default) -->
          <div class="config-section">
            <div class="section-header clickable" @click="configExpanded = !configExpanded">
              <div class="section-header-left">
                <Database :size="20" class="section-icon" />
                <h2 class="section-title">Configuration</h2>
              </div>
              <span class="expand-icon">{{ configExpanded ? '−' : '+' }}</span>
            </div>
            <div v-if="configExpanded && config" class="config-content">
              <pre class="config-json">{{ JSON.stringify(config, null, 2) }}</pre>
            </div>
          </div>
        </div>
      </div>
    </main>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { ArrowRight, Activity, Server, Database, Zap } from 'lucide-vue-next'
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
const configExpanded = ref(false)

let refreshInterval: number | null = null

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

/* Header */
.header {
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
  backdrop-filter: blur(8px);
  position: sticky;
  top: 0;
  z-index: 10;
  background-color: rgba(13, 13, 15, 0.8);
}

.header-content {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 1rem 1.5rem;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.logo {
  width: 2rem;
  height: 2rem;
  border-radius: 0.5rem;
  background: linear-gradient(135deg, rgba(139, 133, 255, 0.2) 0%, rgba(139, 133, 255, 0.05) 100%);
  display: flex;
  align-items: center;
  justify-content: center;
  border: 1px solid rgba(139, 133, 255, 0.2);
}

.logo-icon {
  color: var(--primary);
}

.header-title {
  font-size: 1.25rem;
  font-weight: 500;
  letter-spacing: -0.01em;
  color: var(--foreground);
  margin: 0;
}

.playground-button {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.5rem 1rem;
  border-radius: 0.5rem;
  border: 1px solid var(--border);
  color: var(--foreground);
  text-decoration: none;
  font-size: 0.875rem;
  transition: all 0.2s;
}

.playground-button:hover {
  border-color: rgba(139, 133, 255, 0.5);
}

.arrow-icon {
  transition: transform 0.2s;
}

.playground-button:hover .arrow-icon {
  transform: translateX(2px);
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

/* Config Section */
.config-section {
  background-color: var(--card);
  border: 1px solid var(--border);
  border-radius: 0.75rem;
  padding: 1.5rem;
}

.expand-icon {
  font-size: 1.5rem;
  font-weight: bold;
  color: rgb(161, 161, 170);
}

.config-content {
  margin-top: 1rem;
}

.config-json {
  background-color: #0d0d0f;
  color: #e4e4e7;
  padding: 1rem;
  border-radius: 0.5rem;
  overflow-x: auto;
  font-size: 0.875rem;
  line-height: 1.5;
  font-family: ui-monospace, SFMono-Regular, 'SF Mono', Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace;
}
</style>
