import { createContext } from 'react'
import type { DiagramFence } from '@/features/agent/transcript/plate/streaming-markdown-parse'

/** What a transcript message says about its diagram fences: whether it still
 *  streams and which diagram fence its text leaves open. `null` outside the
 *  transcript, where code blocks keep the composer's behaviour. */
export interface StreamingFence {
  live: boolean
  openFence: DiagramFence | null
}

export const StreamingFenceContext = createContext<StreamingFence | null>(null)
