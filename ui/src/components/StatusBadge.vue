<template>
  <span :class="['status-badge', statusClass]">
    {{ status }}
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  status: string
}>()

const statusClass = computed(() => {
  switch (props.status.toLowerCase()) {
    case 'healthy':
      return 'status-healthy'
    case 'active':
      return 'status-active'
    case 'disabled':
      return 'status-disabled'
    case 'warning':
      return 'status-warning'
    case 'error':
    case 'unhealthy':
      return 'status-error'
    default:
      return 'status-default'
  }
})
</script>

<style scoped>
.status-badge {
  display: inline-flex;
  align-items: center;
  padding: 0.25rem 0.625rem;
  border-radius: 0.375rem;
  font-size: 0.75rem;
  border: 1px solid;
  text-transform: capitalize;
}

.status-healthy {
  background-color: rgba(16, 185, 129, 0.1);
  color: rgb(52, 211, 153);
  border-color: rgba(16, 185, 129, 0.2);
}

.status-active {
  background-color: rgba(59, 130, 246, 0.1);
  color: rgb(96, 165, 250);
  border-color: rgba(59, 130, 246, 0.2);
}

.status-disabled {
  background-color: rgba(255, 255, 255, 0.05);
  color: rgb(161, 161, 170);
  border-color: rgba(255, 255, 255, 0.08);
}

.status-warning {
  background-color: rgba(245, 158, 11, 0.1);
  color: rgb(251, 191, 36);
  border-color: rgba(245, 158, 11, 0.2);
}

.status-error {
  background-color: rgba(239, 68, 68, 0.1);
  color: rgb(248, 113, 113);
  border-color: rgba(239, 68, 68, 0.2);
}

.status-default {
  background-color: rgba(255, 255, 255, 0.05);
  color: rgb(161, 161, 170);
  border-color: rgba(255, 255, 255, 0.08);
}
</style>
