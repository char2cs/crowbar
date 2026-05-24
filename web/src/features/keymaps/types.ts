// Stub
export interface Keybinding {
  command: string
  key: string
  when?: string
  enabled?: boolean
  source?: "user" | "default" | "extension" | "preset"
  args?: unknown
}
export interface KeymapContext {
  [key: string]: boolean | string | undefined
}
export type KeybindingPreset = "default" | "vim" | "emacs" | "none" | "vscode" | "jetbrains" | "sublime" | "xcode" | "atom" | "zed"
export interface Command {
  id: string
  title: string
  description?: string
  keybinding?: string
  category?: string
  when?: string
  handler?: () => void | Promise<void>
}
