import { shellConfig } from './shell-config'
import type { ShellMenuEntry } from './shell-menu-entry'

const MENU_PATH = '/__crowbar/menu'

/** Whether this page can ask the shell for a native menu. */
export function canAskShellMenu(): boolean {
  return shellConfig() !== null
}

/** Pops `entries` up at `at` (page pixels) as the desktop's own menu; resolves to the chosen
 *  entry's index, then its index in each submenu it lies in, or `null` when dismissed. */
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
