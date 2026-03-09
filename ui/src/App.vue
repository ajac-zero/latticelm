<template>
  <div id="app">
    <header v-if="user" class="app-header">
      <div class="header-content">
        <div class="logo">
          <router-link to="/" class="logo-link">LLM Gateway</router-link>
        </div>
        <nav class="nav">
          <router-link to="/chat" class="nav-link">Chat</router-link>
          <router-link v-if="user.is_admin" to="/" class="nav-link">Dashboard</router-link>
        </nav>
        <div class="user-menu">
          <span class="user-email">{{ user.email }}</span>
          <button @click="handleLogout" class="logout-btn">Logout</button>
        </div>
      </div>
    </header>
    <main class="main-content">
      <router-view />
    </main>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { getCurrentUser, logout, type User } from './auth'

const user = ref<User | null>(null)

onMounted(async () => {
  user.value = await getCurrentUser()
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
  background-color: var(--card);
  border-bottom: 1px solid var(--border);
  padding: 1rem 2rem;
}

.header-content {
  max-width: 1400px;
  margin: 0 auto;
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.logo {
  font-size: 1.25rem;
  font-weight: 600;
}

.logo-link {
  color: var(--primary);
  text-decoration: none;
}

.logo-link:hover {
  opacity: 0.8;
}

.nav {
  display: flex;
  gap: 1.5rem;
}

.nav-link {
  color: var(--foreground);
  text-decoration: none;
  padding: 0.5rem 1rem;
  border-radius: 0.375rem;
  transition: background-color 0.2s;
}

.nav-link:hover {
  background-color: var(--muted);
}

.nav-link.router-link-active {
  background-color: var(--secondary);
  color: var(--primary);
}

.user-menu {
  display: flex;
  align-items: center;
  gap: 1rem;
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
}
</style>
