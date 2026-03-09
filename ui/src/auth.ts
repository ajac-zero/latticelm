export interface User {
  email: string
  name: string
  is_admin: boolean
}

export interface Config {
  auth_enabled: boolean
}

let cachedUser: User | null | undefined = undefined
let cachedConfig: Config | null = null

/**
 * Load configuration from the server.
 * This determines whether auth is enabled.
 */
export async function loadConfig(): Promise<Config> {
  if (cachedConfig !== null) {
    return cachedConfig
  }

  try {
    const response = await fetch('/api/config')
    if (!response.ok) {
      throw new Error('Failed to load config')
    }
    const config = await response.json()
    cachedConfig = config
    return config
  } catch (error) {
    console.error('Failed to load config:', error)
    // Default to auth disabled if config can't be loaded
    cachedConfig = { auth_enabled: false }
    return cachedConfig
  }
}

/**
 * Check if authentication is enabled.
 */
export async function isAuthEnabled(): Promise<boolean> {
  const config = await loadConfig()
  return config.auth_enabled
}

/**
 * Get the currently authenticated user.
 * Returns null if not authenticated or if auth is disabled.
 */
export async function getCurrentUser(): Promise<User | null> {
  // Check if auth is enabled first
  const authEnabled = await isAuthEnabled()
  if (!authEnabled) {
    return null
  }

  // Return cached value if available
  if (cachedUser !== undefined) {
    return cachedUser
  }

  try {
    const response = await fetch('/auth/user', {
      credentials: 'include' // Include session cookie
    })

    if (!response.ok) {
      cachedUser = null
      return null
    }

    const user = await response.json()
    cachedUser = user
    return user
  } catch (error) {
    console.error('Failed to get current user:', error)
    cachedUser = null
    return null
  }
}

/**
 * Clear the cached user (useful after logout).
 */
export function clearUserCache() {
  cachedUser = undefined
}

/**
 * Redirect to login page.
 */
export function login() {
  window.location.href = '/auth/login'
}

/**
 * Logout the current user.
 */
export function logout() {
  clearUserCache()
  window.location.href = '/auth/logout'
}
