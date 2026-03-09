import { apiClient } from './client'
import type { ConfigResponse } from '../types/api'

export const configAPI = {
  async getConfig(): Promise<ConfigResponse> {
    return apiClient.get<ConfigResponse>('/config')
  },
}
