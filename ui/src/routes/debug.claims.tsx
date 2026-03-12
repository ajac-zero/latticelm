import { createFileRoute } from '@tanstack/react-router'
import { useEffect, useState } from 'react'
import { requireAuth } from '../lib/auth'

export const Route = createFileRoute('/debug/claims')({
  beforeLoad: requireAuth,
  component: DebugClaimsPage,
})

function DebugClaimsPage() {
  const [data, setData] = useState<any>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    async function loadClaims() {
      try {
        const response = await fetch('/api/auth/debug/claims', {
          credentials: 'include',
        })

        if (!response.ok) {
          throw new Error('Failed to fetch claims')
        }

        const result = await response.json()
        setData(result.data)
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Unknown error')
      } finally {
        setLoading(false)
      }
    }

    loadClaims()
  }, [])

  if (loading) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <div className="text-lg">Loading claims...</div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <div className="text-lg text-red-500">Error: {error}</div>
      </div>
    )
  }

  return (
    <div className="container mx-auto max-w-4xl p-6">
      <h1 className="mb-6 text-3xl font-bold">JWT Claims Debug</h1>

      <div className="mb-4 rounded-lg border border-border bg-card p-4">
        <div className="mb-2 flex items-center justify-between">
          <span className="text-sm font-medium text-muted-foreground">Auth Mode:</span>
          <span className="font-mono text-sm">{data?.mode}</span>
        </div>
        <div className="flex items-center justify-between">
          <span className="text-sm font-medium text-muted-foreground">Is Admin:</span>
          <span className={`font-mono text-sm ${data?.is_admin ? 'text-green-500' : 'text-red-500'}`}>
            {String(data?.is_admin)}
          </span>
        </div>
      </div>

      <div className="rounded-lg border border-border bg-card p-4">
        <h2 className="mb-4 text-lg font-semibold">All Claims:</h2>
        <pre className="overflow-x-auto rounded bg-muted p-4 text-sm">
          {JSON.stringify(data?.claims, null, 2)}
        </pre>
      </div>

      <div className="mt-6 rounded-lg border border-yellow-500/50 bg-yellow-500/10 p-4">
        <p className="text-sm text-yellow-200">
          <strong>Looking for the admin role?</strong> Check the claims above for fields like{' '}
          <code className="rounded bg-black/20 px-1">role</code>,{' '}
          <code className="rounded bg-black/20 px-1">roles</code>,{' '}
          <code className="rounded bg-black/20 px-1">groups</code>, or{' '}
          <code className="rounded bg-black/20 px-1">public_metadata</code>.
        </p>
        <p className="mt-2 text-sm text-yellow-200">
          The backend looks for the value <code className="rounded bg-black/20 px-1">"admin"</code> in these fields.
        </p>
      </div>
    </div>
  )
}
