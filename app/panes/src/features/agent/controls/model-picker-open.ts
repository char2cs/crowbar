import { useState } from 'react'
import { create } from 'zustand'

interface ModelPickerOpenState {
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

export function useModelPickerOpen(chatId: string | undefined) {
  const [own, setOwn] = useState(false)
  const shared = useModelPickerOpenStore((s) => chatId !== undefined && s.openFor === chatId)
  const setShared = useModelPickerOpenStore((s) => s.setOpen)
  if (chatId === undefined) return [own, setOwn] as const
  return [shared, (next: boolean) => setShared(chatId, next)] as const
}

export function openModelPicker(chatId: string) {
  useModelPickerOpenStore.getState().setOpen(chatId, true)
}
