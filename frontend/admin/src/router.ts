import { createRouter, createWebHistory } from 'vue-router'
import Dashboard from './views/Dashboard.vue'
import Chat from './views/Chat.vue'
import Login from './views/Login.vue'

const router = createRouter({
  history: createWebHistory('/admin/'),
  routes: [
    {
      path: '/',
      name: 'dashboard',
      component: Dashboard,
      meta: { requiresAuth: true },
    },
    {
      path: '/chat',
      name: 'chat',
      component: Chat,
      meta: { requiresAuth: true },
    },
    {
      path: '/login',
      name: 'login',
      component: Login,
    },
  ],
})

// Probe a protected endpoint to determine if we have a valid session.
async function isAuthenticated(): Promise<boolean> {
  try {
    const resp = await fetch('/admin/api/v1/system/health', { credentials: 'same-origin' })
    return resp.status !== 401
  } catch {
    return false
  }
}

router.beforeEach(async (to) => {
  if (to.meta.requiresAuth) {
    const authed = await isAuthenticated()
    if (!authed) {
      return { name: 'login' }
    }
  }
  return true
})

export default router
