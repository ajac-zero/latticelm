import { apiClient } from './client'
import type { SystemInfo, HealthCheckResponse } from '../types/api'

export const systemAPI = {
  async getInfo(): Promise<SystemInfo> {
    return apiClient.get<SystemInfo>('/system/info')
  },

  async getHealth(): Promise<HealthCheckResponse> {
    return apiClient.get<HealthCheckResponse>('/system/health')
  },
}
