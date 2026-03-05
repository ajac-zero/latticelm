<template>
  <div class="dashboard">
    <header class="header">
      <h1>LLM Gateway Admin</h1>
    </header>

    <div class="container">
      <div v-if="loading" class="loading">Loading...</div>
      <div v-else-if="error" class="error">{{ error }}</div>
      <div v-else class="grid">
        <!-- System Info Card -->
        <div class="card">
          <h2>System Information</h2>
          <div class="info-grid" v-if="systemInfo">
            <div class="info-item">
              <span class="label">Version:</span>
              <span class="value">{{ systemInfo.version }}</span>
            </div>
            <div class="info-item">
              <span class="label">Platform:</span>
              <span class="value">{{ systemInfo.platform }}</span>
            </div>
            <div class="info-item">
              <span class="label">Go Version:</span>
              <span class="value">{{ systemInfo.go_version }}</span>
            </div>
            <div class="info-item">
              <span class="label">Uptime:</span>
              <span class="value">{{ systemInfo.uptime }}</span>
            </div>
            <div class="info-item">
              <span class="label">Build Time:</span>
              <span class="value">{{ systemInfo.build_time }}</span>
            </div>
            <div class="info-item">
              <span class="label">Git Commit:</span>
              <span class="value code">{{ systemInfo.git_commit }}</span>
            </div>
          </div>
        </div>

        <!-- Health Status Card -->
        <div class="card">
          <h2>Health Status</h2>
          <div v-if="health">
            <div class="health-overall">
              <span class="label">Overall Status:</span>
              <span :class="['badge', health.status]">{{ health.status }}</span>
            </div>
            <div class="health-checks">
              <div v-for="(check, name) in health.checks" :key="name" class="health-check">
                <span class="check-name">{{ name }}:</span>
                <span :class="['badge', check.status]">{{ check.status }}</span>
                <span v-if="check.message" class="check-message">{{ check.message }}</span>
              </div>
            </div>
          </div>
        </div>

        <!-- Providers Card -->
        <div class="card full-width">
          <h2>Providers</h2>
          <div v-if="providers && providers.length > 0" class="providers-grid">
            <div v-for="provider in providers" :key="provider.name" class="provider-card">
              <div class="provider-header">
                <h3>{{ provider.name }}</h3>
                <span :class="['badge', provider.status]">{{ provider.status }}</span>
              </div>
              <div class="provider-info">
                <div class="info-item">
                  <span class="label">Type:</span>
                  <span class="value">{{ provider.type }}</span>
                </div>
                <div class="info-item">
                  <span class="label">Models:</span>
                  <span class="value">{{ provider.models.length }}</span>
                </div>
              </div>
              <div v-if="provider.models.length > 0" class="models-list">
                <span v-for="model in provider.models" :key="model" class="model-tag">
                  {{ model }}
                </span>
              </div>
            </div>
          </div>
          <div v-else class="empty-state">No providers configured</div>
        </div>

        <!-- Config Card -->
        <div class="card full-width collapsible">
          <div class="card-header" @click="configExpanded = !configExpanded">
            <h2>Configuration</h2>
            <span class="expand-icon">{{ configExpanded ? '−' : '+' }}</span>
          </div>
          <div v-if="configExpanded && config" class="config-content">
            <pre class="config-json">{{ JSON.stringify(config, null, 2) }}</pre>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { systemAPI } from '../api/system'
import { configAPI } from '../api/config'
import { providersAPI } from '../api/providers'
import type { SystemInfo, HealthCheckResponse, ConfigResponse, ProviderInfo } from '../types/api'

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
  background-color: #f5f5f5;
}

.header {
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  color: white;
  padding: 2rem;
  box-shadow: 0 2px 4px rgba(0, 0, 0, 0.1);
}

.header h1 {
  font-size: 2rem;
  font-weight: 600;
}

.container {
  max-width: 1400px;
  margin: 0 auto;
  padding: 2rem;
}

.loading,
.error {
  text-align: center;
  padding: 3rem;
  font-size: 1.2rem;
}

.error {
  color: #e53e3e;
}

.grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(400px, 1fr));
  gap: 1.5rem;
}

.card {
  background: white;
  border-radius: 8px;
  padding: 1.5rem;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.1);
}

.full-width {
  grid-column: 1 / -1;
}

.card h2 {
  font-size: 1.25rem;
  font-weight: 600;
  margin-bottom: 1rem;
  color: #2d3748;
}

.info-grid {
  display: grid;
  gap: 0.75rem;
}

.info-item {
  display: flex;
  justify-content: space-between;
  padding: 0.5rem 0;
  border-bottom: 1px solid #e2e8f0;
}

.info-item:last-child {
  border-bottom: none;
}

.label {
  font-weight: 500;
  color: #4a5568;
}

.value {
  color: #2d3748;
}

.code {
  font-family: 'Courier New', monospace;
  font-size: 0.9rem;
}

.badge {
  display: inline-block;
  padding: 0.25rem 0.75rem;
  border-radius: 12px;
  font-size: 0.875rem;
  font-weight: 500;
}

.badge.healthy {
  background-color: #c6f6d5;
  color: #22543d;
}

.badge.unhealthy {
  background-color: #fed7d7;
  color: #742a2a;
}

.badge.active {
  background-color: #bee3f8;
  color: #2c5282;
}

.health-overall {
  display: flex;
  align-items: center;
  gap: 1rem;
  padding: 1rem;
  background-color: #f7fafc;
  border-radius: 6px;
  margin-bottom: 1rem;
}

.health-checks {
  display: grid;
  gap: 0.75rem;
}

.health-check {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.75rem;
  border: 1px solid #e2e8f0;
  border-radius: 6px;
}

.check-name {
  font-weight: 500;
  color: #4a5568;
  text-transform: capitalize;
}

.check-message {
  color: #718096;
  font-size: 0.875rem;
}

.providers-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 1rem;
}

.provider-card {
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  padding: 1rem;
  background-color: #f7fafc;
}

.provider-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0.75rem;
}

.provider-header h3 {
  font-size: 1.125rem;
  font-weight: 600;
  color: #2d3748;
}

.provider-info {
  display: grid;
  gap: 0.5rem;
  margin-bottom: 0.75rem;
}

.models-list {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  margin-top: 0.75rem;
}

.model-tag {
  background-color: #edf2f7;
  color: #4a5568;
  padding: 0.25rem 0.75rem;
  border-radius: 6px;
  font-size: 0.875rem;
}

.empty-state {
  text-align: center;
  padding: 2rem;
  color: #718096;
}

.collapsible .card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  cursor: pointer;
  user-select: none;
}

.expand-icon {
  font-size: 1.5rem;
  font-weight: bold;
  color: #4a5568;
}

.config-content {
  margin-top: 1rem;
}

.config-json {
  background-color: #2d3748;
  color: #e2e8f0;
  padding: 1rem;
  border-radius: 6px;
  overflow-x: auto;
  font-size: 0.875rem;
  line-height: 1.5;
}
</style>
