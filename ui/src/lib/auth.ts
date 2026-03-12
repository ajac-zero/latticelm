import type { User } from './api/types'
import { redirect } from '@tanstack/react-router'

export interface AuthSession {
  auth_enabled: boolean
  oidc_enabled: boolean
  authenticated: boolean
  mode: 'none' | 'token' | 'oidc'
  user?: User & { id?: string; name?: string }
}

const defaultAuthSession: AuthSession = {
  auth_enabled: false,
  oidc_enabled: false,
  authenticated: false,
  mode: 'none',
}

export async function getAuthSession(): Promise<AuthSession> {
  try {
    const response = await fetch('/api/auth/session', {
      credentials: 'include',
    })

    if (!response.ok) {
      return defaultAuthSession
    }

    const payload = await response.json()
    const data = payload?.data

    if (!data || typeof data !== 'object') {
      return defaultAuthSession
    }

    return {
      auth_enabled: data.auth_enabled === true,
      oidc_enabled: data.oidc_enabled === true,
      authenticated: data.authenticated === true,
      mode:
        data.mode === 'token' || data.mode === 'oidc' || data.mode === 'none'
          ? data.mode
          : 'none',
      user: data.user,
    }
  } catch {
    return defaultAuthSession
  }
}

export async function login(token: string): Promise<void> {
  const response = await fetch('/api/auth/token-login', {
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

export async function startOIDCLogin(): Promise<void> {
  const response = await fetch('/api/auth/oidc/login', {
    method: 'POST',
    credentials: 'include',
  })

  if (!response.ok) {
    const data = await response.json().catch(() => ({}))
    throw new Error(data?.error?.message || 'Unable to start OIDC login')
  }

  const payload = await response.json().catch(() => ({}))
  const authorizationURL = payload?.data?.authorization_url
  if (!authorizationURL || typeof authorizationURL !== 'string') {
    throw new Error('OIDC login response missing authorization URL')
  }

  window.location.href = authorizationURL
}

export async function logout(): Promise<void> {
  await fetch('/api/auth/logout', {
    method: 'POST',
    credentials: 'include',
  })
  window.location.href = '/auth/login'
}

export async function getCurrentUser(): Promise<User | null> {
  const session = await getAuthSession()
  if (!session.authenticated || !session.user) {
    return null
  }
  return session.user
}

export async function isAuthEnabled(): Promise<boolean> {
  const session = await getAuthSession()
  return session.auth_enabled
}

export async function isOIDCEnabled(): Promise<boolean> {
  const session = await getAuthSession()
  return session.oidc_enabled
}

/**
 * Auth guard for protected routes.
 * Checks if auth is enabled, and if so, redirects to login if user is not authenticated.
 */
export async function requireAuth() {
  try {
    const session = await getAuthSession()

    if (session.auth_enabled && !session.authenticated) {
      throw redirect({ to: '/auth/login', search: { session_expired: false } })
    }
  } catch (error) {
    // If it's already a redirect, re-throw it
    if (error && typeof error === 'object' && 'isRedirect' in error) {
      throw error
    }
    // Otherwise, allow navigation (fail open if config fetch fails)
  }
}
