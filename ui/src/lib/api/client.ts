import type { APIResponse } from './types'

class APIClient {
  private baseURL = '/api/v1'

  private async request<T>(url: string, options?: RequestInit): Promise<T> {
    const response = await fetch(`${this.baseURL}${url}`, {
      ...options,
      credentials: 'include',
      headers: {
        'Content-Type': 'application/json',
        ...options?.headers,
      },
    })

    if (response.status === 401) {
      localStorage.removeItem('auth_token')
      window.location.href = '/auth/login?session_expired=true'
      throw new Error('Session expired')
    }

    if (!response.ok) {
      if (response.status === 401) {
        window.location.href = '/auth/login'
      }
      throw new Error(`HTTP error! status: ${response.status}`)
    }

    const data = await response.json() as APIResponse<T>

    if (data.success && data.data) {
      return data.data
    }

    throw new Error(data.error?.message || 'Unknown error')
  }

  async get<T>(url: string): Promise<T> {
    return this.request<T>(url)
  }

  async post<T>(url: string, body: any): Promise<T> {
    return this.request<T>(url, {
      method: 'POST',
      body: JSON.stringify(body),
    })
  }

  async put<T>(url: string, body: any): Promise<T> {
    return this.request<T>(url, {
      method: 'PUT',
      body: JSON.stringify(body),
    })
  }

  async patch<T>(url: string, body: any): Promise<T> {
    return this.request<T>(url, {
      method: 'PATCH',
      body: JSON.stringify(body),
    })
  }

  async delete<T>(url: string): Promise<T> {
    return this.request<T>(url, {
      method: 'DELETE',
    })
  }
}

export const apiClient = new APIClient()
