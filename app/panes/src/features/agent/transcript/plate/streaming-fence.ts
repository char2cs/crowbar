import { createContext } from 'react'
import type { DiagramFence } from '@/features/agent/transcript/plate/streaming-markdown-parse'

export interface StreamingFence {
  live: boolean
  openFence: DiagramFence | null
}

export const StreamingFenceContext = createContext<StreamingFence | null>(null)
