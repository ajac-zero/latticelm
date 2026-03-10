import axios, { AxiosInstance } from 'axios'
import type { APIResponse } from '../types/api'

class APIClient {
  private client: AxiosInstance

  constructor() {
    this.client = axios.create({
      baseURL: '/admin/api/v1',
      headers: {
        'Content-Type': 'application/json',
      },
      withCredentials: true,
    })

    // Response interceptor for error handling
    this.client.interceptors.response.use(
      (response) => response,
      (error) => {
        if (error.response?.status === 401) {
          window.location.href = '/admin/login'
        }
        console.error('API Error:', error)
        return Promise.reject(error)
      }
    )
  }

  async get<T>(url: string): Promise<T> {
    const response = await this.client.get<APIResponse<T>>(url)
    if (response.data.success && response.data.data) {
      return response.data.data
    }
    throw new Error(response.data.error?.message || 'Unknown error')
  }

  async post<T>(url: string, data: any): Promise<T> {
    const response = await this.client.post<APIResponse<T>>(url, data)
    if (response.data.success && response.data.data) {
      return response.data.data
    }
    throw new Error(response.data.error?.message || 'Unknown error')
  }
}

export const apiClient = new APIClient()
