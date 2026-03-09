import { createRouter, createWebHistory } from 'vue-router'
import type { RouteLocationNormalized } from 'vue-router'
import Dashboard from './views/Dashboard.vue'
import Chat from './views/Chat.vue'
import { getCurrentUser } from './auth'

const router = createRouter({
  history: createWebHistory('/'),
  routes: [
    {
      path: '/',
      name: 'dashboard',
      component: Dashboard,
      meta: { requiresAdmin: true }
    },
    {
      path: '/chat',
      name: 'chat',
      component: Chat,
      meta: { requiresAuth: true }
    }
  ]
})

// Navigation guard to check authentication and authorization
router.beforeEach(async (to: RouteLocationNormalized) => {
  // Skip auth check for auth routes
  if (to.path.startsWith('/auth/')) {
    return true
  }

  const user = await getCurrentUser()

  // Check if route requires authentication
  if (to.meta.requiresAuth || to.meta.requiresAdmin) {
    if (!user) {
      // Redirect to login
      window.location.href = '/auth/login'
      return false
    }

    // Check if route requires admin access
    if (to.meta.requiresAdmin && !user.is_admin) {
      // Non-admin trying to access dashboard, redirect to chat
      return { name: 'chat' }
    }
  }

  return true
})

export default router
