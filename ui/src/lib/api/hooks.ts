import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiClient } from './client'
import type { SystemInfo, HealthCheckResponse, ConfigResponse, ProviderInfo, ListUsersResponse, UserDetail, UpdateUserRequest, UsageSummaryResponse, UsageTopResponse, UsageTrendsResponse } from './types'

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

export const useConfig = (enabled = true) => {
  return useQuery({
    queryKey: ['config'],
    queryFn: () => apiClient.get<ConfigResponse>('/config'),
    refetchInterval: 30000,
    enabled,
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

// User Management API
const userApi = {
  listUsers: async (params: {
    page?: number
    limit?: number
    role?: string
    status?: string
    search?: string
  }): Promise<ListUsersResponse> => {
    const searchParams = new URLSearchParams()
    if (params.page) searchParams.append('page', String(params.page))
    if (params.limit) searchParams.append('limit', String(params.limit))
    if (params.role) searchParams.append('role', params.role)
    if (params.status) searchParams.append('status', params.status)
    if (params.search) searchParams.append('search', params.search)

    const response = await fetch(`/api/users?${searchParams}`, {
      credentials: 'include',
    })
    if (!response.ok) throw new Error('Failed to fetch users')
    return response.json()
  },

  getUser: async (id: string): Promise<UserDetail> => {
    const response = await fetch(`/api/users/${id}`, {
      credentials: 'include',
    })
    if (!response.ok) throw new Error('Failed to fetch user')
    return response.json()
  },

  updateUser: async (id: string, data: UpdateUserRequest): Promise<UserDetail> => {
    const response = await fetch(`/api/users/${id}`, {
      method: 'PATCH',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(data),
    })
    if (!response.ok) {
      const error = await response.json()
      throw new Error(error.error || 'Failed to update user')
    }
    return response.json()
  },

  deleteUser: async (id: string): Promise<{ message: string; id: string }> => {
    const response = await fetch(`/api/users/${id}`, {
      method: 'DELETE',
      credentials: 'include',
    })
    if (!response.ok) {
      const error = await response.json()
      throw new Error(error.error || 'Failed to delete user')
    }
    return response.json()
  },
}

export const useUsers = (params?: {
  page?: number
  limit?: number
  role?: string
  status?: string
  search?: string
}) => {
  return useQuery({
    queryKey: ['users', params],
    queryFn: () => userApi.listUsers(params || {}),
  })
}

export const useUser = (id: string) => {
  return useQuery({
    queryKey: ['users', id],
    queryFn: () => userApi.getUser(id),
    enabled: !!id,
  })
}

export const useUpdateUser = () => {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: UpdateUserRequest }) =>
      userApi.updateUser(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['users'] })
    },
  })
}

export const useDeleteUser = () => {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => userApi.deleteUser(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['users'] })
    },
  })
}

// Usage Analytics API
const usageApi = {
  summary: async (params: { start: string; end: string }): Promise<UsageSummaryResponse> => {
    const searchParams = new URLSearchParams({ start: params.start, end: params.end })
    const response = await fetch(`/v1/usage/summary?${searchParams}`, { credentials: 'include' })
    if (!response.ok) throw new Error('Failed to fetch usage summary')
    return response.json()
  },

  top: async (params: { start: string; end: string; dimension: string; limit: number }): Promise<UsageTopResponse> => {
    const searchParams = new URLSearchParams({
      start: params.start,
      end: params.end,
      dimension: params.dimension,
      limit: String(params.limit),
    })
    const response = await fetch(`/v1/usage/top?${searchParams}`, { credentials: 'include' })
    if (!response.ok) throw new Error('Failed to fetch top usage')
    return response.json()
  },

  trends: async (params: { start: string; end: string; granularity: string }): Promise<UsageTrendsResponse> => {
    const searchParams = new URLSearchParams({
      start: params.start,
      end: params.end,
      granularity: params.granularity,
    })
    const response = await fetch(`/v1/usage/trends?${searchParams}`, { credentials: 'include' })
    if (!response.ok) throw new Error('Failed to fetch usage trends')
    return response.json()
  },
}

export const useUsageSummary = (params: { start: string; end: string }) => {
  return useQuery({
    queryKey: ['usage', 'summary', params],
    queryFn: () => usageApi.summary(params),
    refetchInterval: 60000,
  })
}

export const useUsageTop = (params: { start: string; end: string; dimension: string; limit: number }) => {
  return useQuery({
    queryKey: ['usage', 'top', params],
    queryFn: () => usageApi.top(params),
    refetchInterval: 60000,
  })
}

export const useUsageTrends = (params: { start: string; end: string; granularity: string }) => {
  return useQuery({
    queryKey: ['usage', 'trends', params],
    queryFn: () => usageApi.trends(params),
    refetchInterval: 60000,
  })
}
