import type { User } from './api/types'

export async function getCurrentUser(): Promise<User | null> {
  try {
    const token = localStorage.getItem('auth_token')
    if (!token) return null

    const response = await fetch('/api/v1/auth/me', {
      headers: {
        Authorization: `Bearer ${token}`,
      },
    })

    if (!response.ok) {
      localStorage.removeItem('auth_token')
      return null
    }

    const data = await response.json()
    return data.data
  } catch {
    return null
  }
}

export async function isAuthEnabled(): Promise<boolean> {
  try {
    const response = await fetch('/api/v1/config')
    const data = await response.json()
    return data.data?.auth?.enabled || false
  } catch {
    return false
  }
}

export function logout() {
  localStorage.removeItem('auth_token')
  window.location.href = '/auth/login'
}
