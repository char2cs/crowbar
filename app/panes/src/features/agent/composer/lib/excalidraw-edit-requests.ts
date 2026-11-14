import { create } from 'zustand'
import type { ParsedExcalidrawScene } from '@/features/agent/composer/plate/attachments/excalidraw-scene'

// How an Edit button on a diagram deep in a transcript reaches the composer
// that owns the takeover, without threading a callback through every Plate node.
interface ExcalidrawEditRequests {
  requests: Record<string, ParsedExcalidrawScene>
}

export const useExcalidrawEditRequests = create<ExcalidrawEditRequests>(() => ({ requests: {} }))

export function requestExcalidrawEdit(chatId: string, scene: ParsedExcalidrawScene) {
  useExcalidrawEditRequests.setState((s) => ({ requests: { ...s.requests, [chatId]: scene } }))
}

export function clearExcalidrawEditRequest(chatId: string) {
  const { requests } = useExcalidrawEditRequests.getState()
  if (!(chatId in requests)) return
  const { [chatId]: _done, ...rest } = requests
  useExcalidrawEditRequests.setState({ requests: rest })
}
