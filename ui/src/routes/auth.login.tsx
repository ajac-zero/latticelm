import { createFileRoute } from '@tanstack/react-router'
import { useState, useEffect } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { getAuthSession, startOIDCLogin } from '../lib/auth'
import { Button } from '#/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '#/components/ui/card'

export const Route = createFileRoute('/auth/login')({
  validateSearch: (search: Record<string, unknown>) => ({
    session_expired: search.session_expired === 'true' || search.session_expired === true,
  }),
  component: LoginPage,
})

function LoginPage() {
  const { session_expired } = Route.useSearch()
  const [error, setError] = useState('')
  const [oidcLoading, setOidcLoading] = useState(false)
  const [checking, setChecking] = useState(true)
  const navigate = useNavigate()

  useEffect(() => {
    let cancelled = false

    async function checkSession() {
      const session = await getAuthSession()

      if (!session.auth_enabled) {
        await navigate({ to: '/dashboard' })
        return
      }

      if (session.authenticated) {
        await navigate({ to: '/dashboard' })
        return
      }

      if (!cancelled) {
        setChecking(false)
      }
    }

    checkSession()

    return () => {
      cancelled = true
    }
  }, [])

  const handleOIDCLogin = async () => {
    setError('')
    setOidcLoading(true)
    try {
      await startOIDCLogin()
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Unable to start SSO login')
      setOidcLoading(false)
    }
  }

  if (checking) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <div className="text-muted-foreground">Loading...</div>
      </div>
    )
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-muted/40">
      <Card className="w-full max-w-md">
        <CardHeader>
          <CardTitle className="text-2xl">LatticeLM</CardTitle>
          <CardDescription>Sign in to continue.</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          {session_expired && (
            <div className="rounded-md bg-yellow-50 p-3 text-sm text-yellow-800 dark:bg-yellow-900/20 dark:text-yellow-400">
              Your session has expired. Please sign in again.
            </div>
          )}

          {error && (
            <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
              {error}
            </div>
          )}

          <Button onClick={handleOIDCLogin} disabled={oidcLoading} size="lg" className="w-full">
            {oidcLoading ? 'Redirecting...' : 'Sign in with SSO'}
          </Button>
        </CardContent>
      </Card>
    </div>
  )
}
