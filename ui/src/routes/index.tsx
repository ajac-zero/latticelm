import { createFileRoute, redirect } from '@tanstack/react-router'
import { requireAuth } from '../lib/auth'

export const Route = createFileRoute('/')({
  beforeLoad: async () => {
    await requireAuth()
    throw redirect({ to: '/dashboard' })
  },
})
