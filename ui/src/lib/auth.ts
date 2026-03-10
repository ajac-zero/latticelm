import type { User } from './api/types'
import { redirect } from '@tanstack/react-router'

export async function login(token: string): Promise<void> {
  const response = await fetch('/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'include',
    body: JSON.stringify({ token }),
  })

  if (!response.ok) {
    const data = await response.json().catch(() => ({}))
    throw new Error(data?.error?.message || 'Login failed')
  }
}

export async function logout(): Promise<void> {
  await fetch('/auth/logout', {
    method: 'POST',
    credentials: 'include',
  })
  window.location.href = '/auth/login'
}

export async function getCurrentUser(): Promise<User | null> {
  try {
    const response = await fetch('/auth/user', {
      credentials: 'include',
    })

    if (!response.ok) {
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

// Probe a protected endpoint to determine if we have a valid session.
async function isAuthenticated(): Promise<boolean> {
  try {
    const response = await fetch('/api/v1/system/health', { credentials: 'include' })
    return response.status !== 401
  } catch {
    return false
  }
}

/**
 * Auth guard for protected routes.
 * Checks if auth is enabled, and if so, redirects to login if user is not authenticated.
 */
export async function requireAuth() {
  try {
    const authEnabled = await isAuthEnabled()

    if (authEnabled) {
      const authed = await isAuthenticated()
      if (!authed) {
        throw redirect({ to: '/auth/login', search: { session_expired: false } })
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
