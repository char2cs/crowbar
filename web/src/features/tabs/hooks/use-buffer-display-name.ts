import { useCallback, useMemo } from 'react'
import { useTerminalStore } from '@/features/terminal/stores/terminal-store'
import { formatDiffBufferLabel } from '@/features/git/utils/diff-buffer-label'
import type { PaneContent } from '@/features/panes/types/pane-content'
import { calculateDisplayNames } from '../utils/path-shortener'

interface UseBufferDisplayNameOptions {
  buffers: PaneContent[]
  rootFolderPath: string | undefined
}

/**
 * Returns `getBufferDisplayName(buffer)` — a stable callback that derives the
 * human-readable tab label from buffer metadata, terminal session state and
 * path-shortener disambiguation.
 */
export function useBufferDisplayName({ buffers, rootFolderPath }: UseBufferDisplayNameOptions) {
  const terminalSessions = useTerminalStore((state) => state.sessions)

  const displayNames = useMemo(
    () => calculateDisplayNames(buffers, rootFolderPath),
    [buffers, rootFolderPath],
  )

  const isUsefulTerminalTitle = useCallback((title?: string) => {
    if (!title) return false
    const trimmed = title.trim()
    if (!trimmed || trimmed === 'Default Terminal') return false
    if (trimmed.length > 28) return false
    if (trimmed.includes('@')) return false
    if (trimmed.includes('/') || trimmed.includes('\\')) return false

    for (const char of trimmed) {
      const code = char.charCodeAt(0)
      if ((code >= 0 && code <= 31) || code === 127 || code === 155) {
        return false
      }
    }

    return true
  }, [])

  const getDirectoryLabel = useCallback((directory?: string) => {
    if (!directory) return ''
    const normalized = directory.replace(/[\\/]+$/, '')
    return normalized.split(/[\\/]/).pop() || directory
  }, [])

  const getCommandLabel = useCallback((command?: string) => {
    if (!command) return ''
    const firstSegment = command.trim().split(/\s+/)[0]
    return firstSegment?.split(/[\\/]/).pop() || ''
  }, [])

  const getBufferDisplayName = useCallback(
    (buffer: PaneContent) => {
      if (buffer.type === 'terminal') {
        const session = terminalSessions.get(buffer.sessionId)
        if (session?.customName) {
          return session.name?.trim() || buffer.name
        }

        const title = session?.title?.trim()
        if (isUsefulTerminalTitle(title)) return title!

        const commandLabel = getCommandLabel(buffer.initialCommand)
        if (commandLabel) return commandLabel

        const dirLabel = getDirectoryLabel(session?.currentDirectory || buffer.workingDirectory)
        if (dirLabel) return dirLabel
      }

      if (buffer.type === 'diff') {
        return formatDiffBufferLabel(displayNames.get(buffer.id) || buffer.name, buffer.path)
      }

      return displayNames.get(buffer.id) ?? buffer.name
    },
    [displayNames, getCommandLabel, getDirectoryLabel, isUsefulTerminalTitle, terminalSessions],
  )

  return getBufferDisplayName
}
