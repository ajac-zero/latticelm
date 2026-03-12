import { useQuery } from '@tanstack/react-query'
import { apiClient } from './client'
import type { SystemInfo, HealthCheckResponse, ConfigResponse, ProviderInfo } from './types'

export const useSystemInfo = () => {
  return useQuery({
    queryKey: ['system', 'info'],
    queryFn: () => apiClient.get<SystemInfo>('/system/info'),
    refetchInterval: 30000, // Refresh every 30 seconds
  })
}

export const useHealth = () => {
  return useQuery({
    queryKey: ['system', 'health'],
    queryFn: () => apiClient.get<HealthCheckResponse>('/system/health'),
    refetchInterval: 30000,
  })
}

export const useConfig = () => {
  return useQuery({
    queryKey: ['config'],
    queryFn: () => apiClient.get<ConfigResponse>('/config'),
    refetchInterval: 30000,
  })
}

export const useProviders = () => {
  return useQuery({
    queryKey: ['providers'],
    queryFn: () => apiClient.get<ProviderInfo[]>('/providers'),
    refetchInterval: 30000,
  })
}

export const useModels = () => {
  return useQuery({
    queryKey: ['models'],
    queryFn: async () => {
      const response = await fetch('/v1/models', {
        credentials: 'include',
      })
      const data = await response.json()
      return data.data || []
    },
  })
}
