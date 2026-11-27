import { shellConfig } from './shell-config'
import type { ShellMenuEntry } from './shell-menu-entry'

const MENU_PATH = '/__crowbar/menu'

export function canAskShellMenu(): boolean {
  return shellConfig() !== null
}

export async function popShellMenu(
  entries: ShellMenuEntry[],
  at: { x: number; y: number },
): Promise<number[] | null> {
  const config = shellConfig()
  if (!config) throw new Error('popShellMenu: this page is not hosted by the shell')
  const response = await fetch(`${config.api}${MENU_PATH}`, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ entries, x: at.x, y: at.y }),
  })
  if (!response.ok) throw new Error(`popShellMenu: the shell answered ${response.status}`)
  const { path } = (await response.json()) as { path: number[] | null }
  return path
}
