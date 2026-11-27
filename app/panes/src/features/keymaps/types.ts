export type CommandCategory = 'Panes' | 'Tabs' | 'Editor' | 'Navigation' | 'Chats'

export interface Command {
  id: string
  label: string
  category: CommandCategory
  defaultChord: string
  liveEditable: boolean
}

export type KeymapPresetId = 'default' | 'compact'

export interface KeymapPreset {
  id: KeymapPresetId
  label: string
  populated: boolean
  bindings: Record<string, string>
}

export type KeymapOverrides = Record<string, string>

export interface EffectiveBinding {
  commandId: string
  chord: string
  source: 'default' | 'preset' | 'user'
}
