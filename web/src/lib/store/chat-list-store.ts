import { create } from 'zustand'
import { createLoadableSlice, type LoadableSlice } from '@/lib/store/loadable-slice'
import { apiFetch } from '@/lib/api'
import type { ChatDto } from '@/lib/api/chat'

export const useChatListStore = create<LoadableSlice<ChatDto[], [string]>>()((set, get) =>
  createLoadableSlice<ChatDto[], [string]>({
    store: 'chats-data',
    fetcher: (wsId: string) =>
      apiFetch<ChatDto[]>(`/v0/workspaces/${encodeURIComponent(wsId)}/chats`),
    cacheKey: (wsId: string) => wsId,
    wsEndpoint: (wsId: string) => `/v0/ws/chats?wsId=${encodeURIComponent(wsId)}`,
  })(set, get),
)
