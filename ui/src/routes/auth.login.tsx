import { createFileRoute } from '@tanstack/react-router'
import { useState, useEffect } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { login, isOIDCEnabled } from '../lib/auth'

export const Route = createFileRoute('/auth/login')({
  validateSearch: (search: Record<string, unknown>) => ({
    session_expired: search.session_expired === 'true' || search.session_expired === true,
  }),
  component: LoginPage,
})

function LoginPage() {
  const { session_expired } = Route.useSearch()
  const [token, setToken] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [checkingOIDC, setCheckingOIDC] = useState(true)
  const navigate = useNavigate()

  useEffect(() => {
    async function checkOIDC() {
      const oidcEnabled = await isOIDCEnabled()
      if (oidcEnabled) {
        // For OIDC, navigate directly to the backend's /auth/login endpoint
        // This sets cookies properly and redirects to Clerk
        // In dev mode, we need to navigate to the backend port directly
        const isDev = import.meta.env.DEV
        if (isDev) {
          // In dev, navigate to backend port (8080) to set cookies properly
          window.location.href = 'http://localhost:8080/auth/login'
        } else {
          // In production, use relative path
          window.location.href = '/auth/login'
        }
      } else {
        setCheckingOIDC(false)
      }
    }
    checkOIDC()
  }, [])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')

    const trimmedToken = token.trim()
    if (!trimmedToken) {
      setError('Please enter a token')
      return
    }

    setLoading(true)
    try {
      await login(trimmedToken)
      navigate({ to: '/dashboard' })
    } catch (err: any) {
      setError(err.message || 'Login failed')
    } finally {
      setLoading(false)
    }
  }

  if (checkingOIDC) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-gray-50 dark:bg-gray-900">
        <div className="text-lg">Redirecting to login...</div>
      </div>
    )
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-gray-50 dark:bg-gray-900">
      <div className="w-full max-w-md rounded-lg bg-white p-8 shadow-lg dark:bg-gray-800">
        {session_expired && (
          <div className="mb-4 rounded-md bg-yellow-50 p-3 text-sm text-yellow-800 dark:bg-yellow-900/20 dark:text-yellow-400">
            Your session has expired. Please sign in again.
          </div>
        )}

        <div className="mb-6">
          <h1 className="text-2xl font-semibold text-gray-900 dark:text-gray-100">
            LLM Gateway Admin
          </h1>
          <p className="mt-2 text-sm text-gray-600 dark:text-gray-400">
            Authentication is required to access the admin panel.
          </p>
        </div>

        <form onSubmit={handleSubmit} className="space-y-5">
          <div>
            <label
              htmlFor="token"
              className="block text-sm font-medium text-gray-700 dark:text-gray-300"
            >
              Bearer Token
            </label>
            <textarea
              id="token"
              value={token}
              onChange={(e) => setToken(e.target.value)}
              rows={5}
              className="mt-1 block w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm shadow-sm placeholder-gray-400 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700 dark:text-gray-100 dark:placeholder-gray-500"
              placeholder="Paste your JWT token here..."
              required
            />
          </div>

          {error && (
            <div className="rounded-md bg-red-50 p-3 text-sm text-red-800 dark:bg-red-900/20 dark:text-red-400">
              {error}
            </div>
          )}

          <button
            type="submit"
            disabled={!token.trim() || loading}
            className="w-full rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-blue-500 dark:hover:bg-blue-600"
          >
            {loading ? 'Signing in...' : 'Sign In'}
          </button>
        </form>
      </div>
    </div>
  )
}
