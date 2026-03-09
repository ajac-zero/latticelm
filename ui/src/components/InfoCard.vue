<template>
  <div class="info-card">
    <div class="card-header">
      <component :is="icon" class="header-icon" :size="20" />
      <h2 class="card-title">{{ title }}</h2>
    </div>
    <div class="card-content">
      <slot></slot>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { Component } from 'vue'

defineProps<{
  title: string
  icon: Component
}>()
</script>

<style scoped>
.info-card {
  background-color: #16161a;
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 0.75rem;
  padding: 1.5rem;
  transition: border-color 0.2s;
}

.info-card:hover {
  border-color: rgba(139, 133, 255, 0.2);
}

.card-header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-bottom: 1.5rem;
}

.header-icon {
  color: rgb(161, 161, 170);
}

.card-title {
  font-size: 1.125rem;
  font-weight: 500;
  letter-spacing: -0.01em;
  color: #e4e4e7;
  margin: 0;
}

.card-content {
  color: #e4e4e7;
}
</style>
