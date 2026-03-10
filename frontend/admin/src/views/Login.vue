<template>
  <div class="login-page">
    <div class="login-card">
      <h1>LLM Gateway Admin</h1>
      <p class="subtitle">Paste your JWT token to continue</p>
      <form @submit.prevent="submit">
        <textarea
          v-model="token"
          class="token-input"
          placeholder="eyJ..."
          rows="6"
          required
        ></textarea>
        <p v-if="error" class="error">{{ error }}</p>
        <button type="submit" class="btn-login" :disabled="loading">
          {{ loading ? 'Signing in...' : 'Sign In' }}
        </button>
      </form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { authAPI } from '../api/auth'

const router = useRouter()
const token = ref('')
const error = ref<string | null>(null)
const loading = ref(false)

async function submit() {
  error.value = null
  loading.value = true
  try {
    await authAPI.login(token.value.trim())
    await router.push('/')
  } catch (err: any) {
    error.value = err.message || 'Login failed'
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.login-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
}

.login-card {
  background: white;
  border-radius: 12px;
  padding: 2.5rem;
  width: 100%;
  max-width: 480px;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.2);
}

h1 {
  font-size: 1.5rem;
  font-weight: 700;
  color: #2d3748;
  margin-bottom: 0.5rem;
}

.subtitle {
  color: #718096;
  margin-bottom: 1.5rem;
  font-size: 0.9rem;
}

.token-input {
  width: 100%;
  padding: 0.75rem;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  font-family: 'Courier New', monospace;
  font-size: 0.8rem;
  resize: vertical;
  color: #2d3748;
  box-sizing: border-box;
  margin-bottom: 1rem;
}

.token-input:focus {
  outline: none;
  border-color: #667eea;
  box-shadow: 0 0 0 3px rgba(102, 126, 234, 0.15);
}

.error {
  color: #e53e3e;
  font-size: 0.875rem;
  margin-bottom: 1rem;
}

.btn-login {
  width: 100%;
  padding: 0.75rem;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  color: white;
  border: none;
  border-radius: 8px;
  font-size: 1rem;
  font-weight: 600;
  cursor: pointer;
}

.btn-login:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}
</style>
