import { create } from 'zustand'
import { createLoadableSlice, type LoadableSlice } from '@/lib/store/loadable-slice'
import { apiFetch } from '@/lib/api'
import type { Repo } from '@/lib/store/sidebar'

export const useWorkspaceListStore = create<LoadableSlice<Repo[], []>>()((set, get) =>
  createLoadableSlice<Repo[], []>({
    store: 'workspaces-data',
    fetcher: () => apiFetch<Repo[]>('/api/v0/workspaces'),
    cacheKey: () => 'workspaces',
    wsEndpoint: () => '/api/v0/ws/workspaces',
  })(set, get),
)
