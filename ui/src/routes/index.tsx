import { createFileRoute, redirect } from '@tanstack/react-router'
import { requireAuth } from '../lib/auth'

export const Route = createFileRoute('/')({
  beforeLoad: async () => {
    const { session } = await requireAuth()
    // Redirect to dashboard if admin, otherwise to chat
    const destination = session.user?.is_admin ? '/dashboard' : '/chat'
    throw redirect({ to: destination })
  },
})
