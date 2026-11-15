import { shellConfig } from '@/sdk/shell-config'

/** Whether this page was loaded by the GPUI shell, whose loopback relay carries its sockets. */
export function inShell(): boolean {
  return shellConfig() !== null
}
