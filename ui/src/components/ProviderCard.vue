<template>
  <div class="provider-card">
    <div class="provider-header">
      <h3 class="provider-name">{{ name }}</h3>
      <StatusBadge :status="status" />
    </div>

    <div class="provider-info">
      <div class="info-row">
        <span class="info-label">Type:</span>
        <span class="info-value mono">{{ type }}</span>
      </div>

      <div v-if="endpoint" class="info-row">
        <span class="info-label">Endpoint:</span>
        <span class="info-value mono endpoint" :title="endpoint">{{ endpoint }}</span>
      </div>

      <div v-if="apiKey" class="info-row">
        <span class="info-label">API Key:</span>
        <div class="api-key-container">
          <span class="info-value mono">{{ showApiKey ? apiKey : '••••••••••••' }}</span>
          <button @click="showApiKey = !showApiKey" class="eye-button">
            <component :is="showApiKey ? EyeOff : Eye" :size="14" />
          </button>
        </div>
      </div>

      <div class="info-row">
        <span class="info-label">Models:</span>
        <span class="info-value">{{ modelsCount }}</span>
      </div>

      <div v-if="models.length > 0" class="models-section">
        <div v-for="(model, index) in models" :key="index" class="model-tag">
          {{ model }}
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { Eye, EyeOff } from 'lucide-vue-next'
import StatusBadge from './StatusBadge.vue'

defineProps<{
  name: string
  status: string
  type: string
  endpoint?: string
  apiKey?: string
  modelsCount: number
  models: string[]
}>()

const showApiKey = ref(false)
</script>

<style scoped>
.provider-card {
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 0.5rem;
  padding: 1.25rem;
  background-color: rgba(30, 30, 36, 0.5);
  transition: all 0.2s;
}

.provider-card:hover {
  border-color: rgba(139, 133, 255, 0.3);
}

.provider-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 1rem;
}

.provider-name {
  font-size: 1rem;
  font-weight: 500;
  color: #e4e4e7;
  margin: 0;
}

.provider-info {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.info-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.info-label {
  font-size: 0.875rem;
  color: rgb(161, 161, 170);
}

.info-value {
  font-size: 0.875rem;
  color: #e4e4e7;
}

.info-value.mono {
  font-family: ui-monospace, SFMono-Regular, 'SF Mono', Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace;
}

.models-section {
  margin-top: 0.75rem;
  padding-top: 0.75rem;
  border-top: 1px solid rgba(255, 255, 255, 0.08);
}

.model-tag {
  padding: 0.5rem 0.75rem;
  background-color: rgba(13, 13, 15, 0.5);
  border-radius: 0.375rem;
  font-size: 0.875rem;
  font-family: ui-monospace, SFMono-Regular, 'SF Mono', Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace;
  color: rgb(161, 161, 170);
  transition: background-color 0.2s;
}

.model-tag:hover {
  background-color: rgba(13, 13, 15, 0.8);
}

.endpoint {
  max-width: 250px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 0.75rem;
  color: rgba(228, 228, 231, 0.8);
}

.api-key-container {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.eye-button {
  padding: 0.25rem;
  background: transparent;
  border: none;
  cursor: pointer;
  color: rgb(161, 161, 170);
  display: flex;
  align-items: center;
  border-radius: 0.25rem;
  transition: all 0.2s;
}

.eye-button:hover {
  background-color: rgba(30, 30, 36, 0.8);
  color: #e4e4e7;
}
</style>
