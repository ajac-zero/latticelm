export interface User {
  email: string
  name: string
  is_admin: boolean
}

let cachedUser: User | null | undefined = undefined

/**
 * Get the currently authenticated user.
 * Returns null if not authenticated.
 */
export async function getCurrentUser(): Promise<User | null> {
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
