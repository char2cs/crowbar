import type { Ref } from 'react'

export interface Terminal {
  id: string
  name: string
  currentDirectory: string
  isActive: boolean
  isPinned?: boolean
  shell?: string
  profileId?: string
  initialCommand?: string
  createdAt: Date
  lastActivity?: Date
  connectionId?: string
  selection?: string
  title?: string
  customName?: boolean
  // Imperative handle ref attached by the terminal component (see terminal.tsx).
  ref?: Ref<unknown>
  splitMode?: boolean
  splitWithId?: string // ID of the terminal to split with
  remoteConnectionId?: string
}
