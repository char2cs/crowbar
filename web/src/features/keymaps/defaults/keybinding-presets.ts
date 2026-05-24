// Stub
import type { Keybinding } from "@/features/keymaps/types"

export const DEFAULT_KEYBINDINGS: Keybinding[] = []
export const VIM_KEYBINDINGS: Keybinding[] = []
export const EMACS_KEYBINDINGS: Keybinding[] = []
export function getKeybindingPreset(_preset: string): Keybinding[] { return [] }

export type KeybindingPreset = "default" | "vim" | "emacs" | "none" | "vscode" | "jetbrains" | "sublime" | "xcode" | "atom" | "zed"

export const keybindingPresetOptions: Array<{ value: KeybindingPreset; label: string }> = [
  { value: "default", label: "Default" },
  { value: "vim", label: "Vim" },
  { value: "emacs", label: "Emacs" },
]

export interface KeybindingPresetCoverageReport {
  isComplete: boolean
  missingCommandIds: string[]
  coveredCommandIds: string[]
}

export function getKeybindingPresetCoverageReport(_preset: string): KeybindingPresetCoverageReport {
  return { isComplete: true, missingCommandIds: [], coveredCommandIds: [] }
}

export interface KeybindingPresetDiffReport {
  changedCommandIds: string[]
  added: Keybinding[]
  removed: Keybinding[]
  modified: Keybinding[]
}

export function getKeybindingPresetDiffReport(_preset: string): KeybindingPresetDiffReport {
  return { changedCommandIds: [], added: [], removed: [], modified: [] }
}

export function isKeybindingPreset(value: unknown): value is KeybindingPreset {
  return value === "default" || value === "vim" || value === "emacs"
}

export function getExportableUserKeybindings(_keybindings: Keybinding[]): Keybinding[] { return _keybindings }
