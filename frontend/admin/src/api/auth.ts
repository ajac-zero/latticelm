export const authAPI = {
  async login(token: string): Promise<void> {
    const resp = await fetch('/admin/api/v1/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'same-origin',
      body: JSON.stringify({ token }),
    })
    if (!resp.ok) {
      const data = await resp.json().catch(() => ({}))
      throw new Error(data?.error?.message || 'Login failed')
    }
  },

  async logout(): Promise<void> {
    await fetch('/admin/api/v1/auth/logout', {
      method: 'POST',
      credentials: 'same-origin',
    })
  },
}
