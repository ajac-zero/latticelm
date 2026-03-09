<template>
  <div class="info-row">
    <span class="info-label">{{ label }}</span>
    <span :class="['info-value', { mono }]">{{ value }}</span>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  label: string
  value: string
  mono?: boolean
}>()
</script>

<style scoped>
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
  color: #e4e4e7;
}

.info-value.mono {
  font-family: ui-monospace, SFMono-Regular, 'SF Mono', Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace;
}
</style>
