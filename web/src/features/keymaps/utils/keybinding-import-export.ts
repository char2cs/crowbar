// Stub
import type { Keybinding, KeybindingPreset } from "@/features/keymaps/types"

export interface KeybindingsExportPayload {
  keybindingPreset?: KeybindingPreset
  keybindings: Keybinding[]
  version?: number
}

export async function importKeybindings(_file: File): Promise<Keybinding[]> { return [] }
export function exportKeybindings(_bindings: Keybinding[]): string { return "[]" }
export function createKeybindingsExportPayload(payload: KeybindingsExportPayload): KeybindingsExportPayload { return payload }
export function getExportableUserKeybindings(_keybindings?: Keybinding[]): Keybinding[] { return _keybindings ?? [] }
export function parseKeybindingsImportJson(_json: string): KeybindingsExportPayload | null {
  try {
    return JSON.parse(_json) as KeybindingsExportPayload
  } catch {
    return null
  }
}
