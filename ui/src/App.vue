<template>
  <div id="app">
    <!-- Header when auth is enabled and user is logged in -->
    <header v-if="authEnabled && user" class="app-header">
      <div class="header-content">
        <div class="header-left">
          <div class="logo">
            <Zap :size="16" class="logo-icon" />
          </div>
          <h1 class="header-title">LLM Gateway Admin</h1>
        </div>
        <div class="header-right">
          <nav class="nav">
            <router-link to="/" class="nav-link">Dashboard</router-link>
            <router-link to="/chat" class="nav-link">
              <span>Playground</span>
              <ArrowRight :size="14" class="arrow-icon" />
            </router-link>
          </nav>
          <div class="user-menu">
            <span class="user-email">{{ user.email }}</span>
            <button @click="handleLogout" class="logout-btn">Logout</button>
          </div>
        </div>
      </div>
    </header>

    <!-- Header when auth is disabled -->
    <header v-else-if="!authEnabled" class="app-header">
      <div class="header-content">
        <div class="header-left">
          <div class="logo">
            <Zap :size="16" class="logo-icon" />
          </div>
          <h1 class="header-title">LLM Gateway Admin</h1>
        </div>
        <nav class="nav">
          <router-link to="/" class="nav-link">Dashboard</router-link>
          <router-link to="/chat" class="nav-link">
            <span>Playground</span>
            <ArrowRight :size="14" class="arrow-icon" />
          </router-link>
        </nav>
      </div>
    </header>

    <main class="main-content">
      <router-view />
    </main>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Zap, ArrowRight } from 'lucide-vue-next'
import { getCurrentUser, logout, isAuthEnabled, type User } from './auth'

const user = ref<User | null>(null)
const authEnabled = ref<boolean>(false)

onMounted(async () => {
  // Load config first to determine if auth is enabled
  authEnabled.value = await isAuthEnabled()

  // Only try to get user if auth is enabled
  if (authEnabled.value) {
    user.value = await getCurrentUser()
  }
})

function handleLogout() {
  logout()
}
</script>

<style>
:root {
  --background: #0d0d0f;
  --foreground: #e4e4e7;
  --card: #16161a;
  --card-foreground: #e4e4e7;
  --primary: #8b85ff;
  --primary-foreground: #fafafa;
  --secondary: #1e1e24;
  --secondary-foreground: #e4e4e7;
  --muted: #28282e;
  --muted-foreground: #a1a1aa;
  --accent: #8b85ff;
  --border: rgba(255, 255, 255, 0.08);
  --input: rgba(255, 255, 255, 0.08);
}

* {
  margin: 0;
  padding: 0;
  box-sizing: border-box;
}

body {
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
  background-color: var(--background);
  color: var(--foreground);
}

#app {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
}

.app-header {
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
  backdrop-filter: blur(8px);
  background-color: rgba(13, 13, 15, 0.8);
  flex-shrink: 0;
}

.header-content {
  max-width: 1400px;
  margin: 0 auto;
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

.header-right {
  display: flex;
  align-items: center;
  gap: 2rem;
}

.nav {
  display: flex;
  gap: 0.5rem;
}

.nav-link {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  color: var(--foreground);
  text-decoration: none;
  padding: 0.5rem 1rem;
  border-radius: 0.5rem;
  border: 1px solid transparent;
  font-size: 0.875rem;
  transition: all 0.2s;
}

.nav-link:hover {
  border-color: rgba(139, 133, 255, 0.3);
}

.nav-link.router-link-active {
  border-color: var(--border);
}

.arrow-icon {
  transition: transform 0.2s;
}

.nav-link:hover .arrow-icon {
  transform: translateX(2px);
}

.user-menu {
  display: flex;
  align-items: center;
  gap: 1rem;
  padding-left: 2rem;
  border-left: 1px solid var(--border);
}

.user-email {
  color: var(--muted-foreground);
  font-size: 0.875rem;
}

.logout-btn {
  background-color: var(--secondary);
  color: var(--foreground);
  border: 1px solid var(--border);
  padding: 0.5rem 1rem;
  border-radius: 0.375rem;
  cursor: pointer;
  font-size: 0.875rem;
  transition: all 0.2s;
}

.logout-btn:hover {
  background-color: var(--muted);
}

.main-content {
  flex: 1;
  min-height: 0;
  position: relative;
}
</style>
