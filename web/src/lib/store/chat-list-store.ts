import { create } from 'zustand'
import { createLoadableSlice, type LoadableSlice } from '@/lib/store/loadable-slice'
import { apiFetch } from '@/lib/api'
import type { ProjectChat } from '@/lib/store/sidebar'

export const useChatListStore = create<LoadableSlice<ProjectChat[], [string]>>()((set, get) =>
  createLoadableSlice<ProjectChat[], [string]>({
    store: 'chats-data',
    fetcher: (wsId: string) =>
      apiFetch<ProjectChat[]>(`/v0/workspaces/${encodeURIComponent(wsId)}/chats`),
    cacheKey: (wsId: string) => wsId,
    wsEndpoint: (wsId: string) => `/v0/ws/chats?wsId=${encodeURIComponent(wsId)}`,
  })(set, get),
)
