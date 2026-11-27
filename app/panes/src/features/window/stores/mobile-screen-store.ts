import { create } from 'zustand'

type MobileScreen = 'chat' | 'ide'

interface MobileScreenState {
  screen: MobileScreen
  chatListOpen: boolean
  filesOpen: boolean
  ack: number
  open: (screen: Exclude<MobileScreen, 'chat'>) => void
  back: () => void
  setChatListOpen: (open: boolean) => void
  setFilesOpen: (open: boolean) => void
  acknowledge: (seq: number) => void
}

export const useMobileScreenStore = create<MobileScreenState>()((set) => ({
  screen: 'chat',
  chatListOpen: false,
  filesOpen: false,
  ack: 0,
  open: (screen) => set({ screen, chatListOpen: false }),
  back: () => set((s) => (s.filesOpen ? { filesOpen: false } : { screen: 'chat' })),
  setChatListOpen: (chatListOpen) => set({ chatListOpen }),
  setFilesOpen: (filesOpen) => set({ filesOpen }),
  acknowledge: (ack) => set({ ack }),
}))
