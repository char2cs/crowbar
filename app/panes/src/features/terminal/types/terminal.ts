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
  ref?: Ref<unknown>
  splitMode?: boolean
  splitWithId?: string
}
