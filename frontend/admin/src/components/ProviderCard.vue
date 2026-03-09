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
import StatusBadge from './StatusBadge.vue'

defineProps<{
  name: string
  status: string
  type: string
  modelsCount: number
  models: string[]
}>()
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
}
</style>
