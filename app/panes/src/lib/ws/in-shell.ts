import { shellConfig } from '@/sdk/shell-config'

export function inShell(): boolean {
  return shellConfig() !== null
}
