import { useState } from 'react'
import { create } from 'zustand'

interface ModelPickerOpenState {
  /** The chat whose model picker is open; one picker is open at a time. */
  openFor: string | null
  setOpen: (chatId: string, open: boolean) => void
}

const useModelPickerOpenStore = create<ModelPickerOpenState>((set) => ({
  openFor: null,
  setOpen: (chatId, open) =>
    set((state) => {
      if (open) return { openFor: chatId }
      return state.openFor === chatId ? { openFor: null } : state
    }),
}))

/** Whether a picker is open, and the way to open or close it. A picker of a chat (`chatId`) shares
 *  its state so a notice can open it; one with no chat keeps its own. */
export function useModelPickerOpen(chatId: string | undefined) {
  const [own, setOwn] = useState(false)
  const shared = useModelPickerOpenStore((s) => chatId !== undefined && s.openFor === chatId)
  const setShared = useModelPickerOpenStore((s) => s.setOpen)
  if (chatId === undefined) return [own, setOwn] as const
  return [shared, (next: boolean) => setShared(chatId, next)] as const
}

/** Opens `chatId`'s model picker from outside the footer, as a notice's "Switch model" does. */
export function openModelPicker(chatId: string) {
  useModelPickerOpenStore.getState().setOpen(chatId, true)
}
