import { create } from 'zustand'

/** What the phone shows full screen: the chat, or the workspace's IDE pushed over it. */
type MobileScreen = 'chat' | 'ide'

interface MobileScreenState {
  screen: MobileScreen
  /** The chat list, revealed from the left edge by the native chrome. */
  chatListOpen: boolean
  /** The IDE's file explorer, over its tabs. */
  filesOpen: boolean
  /** The newest native intent applied. The chrome shows its own motion until it reads this
   *  back, so a push older than its last word never undoes it. */
  ack: number
  open: (screen: Exclude<MobileScreen, 'chat'>) => void
  back: () => void
  setChatListOpen: (open: boolean) => void
  setFilesOpen: (open: boolean) => void
  acknowledge: (seq: number) => void
}

/** The phone shell's navigation, driven by the native chrome's intents; the shell and the
 *  chrome keep no second copy of where the user is. */
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
