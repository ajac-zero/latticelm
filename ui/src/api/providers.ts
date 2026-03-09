import { apiClient } from './client'
import type { ProviderInfo } from '../types/api'

export const providersAPI = {
  async getProviders(): Promise<ProviderInfo[]> {
    return apiClient.get<ProviderInfo[]>('/providers')
  },
}
