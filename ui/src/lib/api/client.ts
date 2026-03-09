import type { APIResponse } from './types'

class APIClient {
  private baseURL = '/api/v1'

  private async request<T>(url: string, options?: RequestInit): Promise<T> {
    const token = localStorage.getItem('auth_token')

    const response = await fetch(`${this.baseURL}${url}`, {
      ...options,
      headers: {
        'Content-Type': 'application/json',
        ...(token && { Authorization: `Bearer ${token}` }),
        ...options?.headers,
      },
    })

    if (!response.ok) {
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
}

export const apiClient = new APIClient()
