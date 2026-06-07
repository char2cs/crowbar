import { create } from 'zustand'
import { createLoadableSlice, type LoadableSlice } from '@/lib/store/loadable-slice'
import { apiFetch } from '@/lib/api'
import type { MarkdownTurn } from '@/features/markdown-chat/types'

export const useChatStore = create<LoadableSlice<MarkdownTurn[], [string, string]>>()((set, get) =>
  createLoadableSlice<MarkdownTurn[], [string, string]>({
    store: 'chat-history',
    fetcher: (wsId: string, stepId: string) =>
      apiFetch<MarkdownTurn[]>(`/v0/markdown-chat/${wsId}/${stepId}`),
    cacheKey: (wsId: string, stepId: string) => `${wsId}:${stepId}`,
  })(set, get),
)
