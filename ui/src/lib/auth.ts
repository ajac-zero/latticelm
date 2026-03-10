import type { User } from './api/types'
import { redirect } from '@tanstack/react-router'

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
    const response = await fetch('/api/config')
    if (!response.ok) {
      return false
    }
    const data = await response.json()
    return data?.data?.auth_enabled === true
  } catch {
    return false
  }
}

export function logout() {
  localStorage.removeItem('auth_token')
  window.location.href = '/auth/login'
}

/**
 * Auth guard for protected routes.
 * Checks if auth is enabled, and if so, redirects to login if user is not authenticated.
 */
export async function requireAuth() {
  try {
    const authEnabled = await isAuthEnabled()

    if (authEnabled) {
      const user = await getCurrentUser()
      if (!user) {
        throw redirect({ to: '/auth/login' })
      }
    }
  } catch (error) {
    // If it's already a redirect, re-throw it
    if (error && typeof error === 'object' && 'isRedirect' in error) {
      throw error
    }
    // Otherwise, allow navigation (fail open if config fetch fails)
  }
}
