<template>
  <div class="health-item">
    <StatusBadge :status="status" />
    <div class="health-item-content">
      <div class="health-item-label">{{ label }}</div>
      <div v-if="description" class="health-item-description">{{ description }}</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import StatusBadge from './StatusBadge.vue'

defineProps<{
  label: string
  status: string
  description?: string
}>()
</script>

<style scoped>
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
  color: #e4e4e7;
  text-transform: capitalize;
}

.health-item-description {
  font-size: 0.75rem;
  color: rgb(161, 161, 170);
}
</style>
