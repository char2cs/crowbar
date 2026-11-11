import { useCallback, useMemo } from 'react'
import { useShallow } from 'zustand/react/shallow'
import { useTerminalStore } from '@/features/terminal/stores/terminal-store'
import type { PaneContent } from '@/features/panes/types/pane-content'
import { selectRepos, useSelect } from '@/core/entities/select'
import { workspaceNames } from '@/lib/workspace/workspace-branch'
import { calculateDisplayNames } from '../utils/path-shortener'

interface UseBufferDisplayNameOptions {
  buffers: PaneContent[]
}

// Joins each tuple's fields. A control character, not a space: titles and paths contain spaces,
// so two different session states could join to the same string and fool the shallow compare.
const FIELD_SEP = ''

interface TerminalSessionFields {
  customName: boolean
  name: string
  title: string
  currentDirectory: string
}

/** A stable `getBufferDisplayName(buffer)` deriving each tab's label from buffer metadata,
 *  terminal session state and path-shortener disambiguation. */
export function useBufferDisplayName({ buffers }: UseBufferDisplayNameOptions) {
  // Project only the session fields a label reads into flat strings: the `sessions` Map is
  // reallocated on every prompt redraw or `cd`, and useShallow ignores unrelated changes.
  const sessionFieldTuples = useTerminalStore(
    useShallow((state) =>
      buffers
        .filter((b): b is Extract<PaneContent, { type: 'terminal' }> => b.type === 'terminal')
        .map((b) => {
          const s = state.sessions.get(b.sessionId)
          return [
            b.sessionId,
            s?.customName ? '1' : '0',
            s?.name ?? '',
            s?.title ?? '',
            s?.currentDirectory ?? '',
          ].join(FIELD_SEP)
        }),
    ),
  )

  const terminalSessionFields = useMemo(() => {
    const map = new Map<string, TerminalSessionFields>()
    for (const tuple of sessionFieldTuples) {
      const [sessionId, customName, name, title, currentDirectory] = tuple.split(FIELD_SEP)
      map.set(sessionId, { customName: customName === '1', name, title, currentDirectory })
    }
    return map
  }, [sessionFieldTuples])

  const workspaceIds = useMemo(() => [...new Set(buffers.map((b) => b.workspaceId))], [buffers])
  const names = useSelect(
    useShallow((s) => [...workspaceNames(selectRepos(s), workspaceIds).values()]),
  )
  const displayNames = useMemo(
    () => calculateDisplayNames(buffers, new Map(workspaceIds.map((id, i) => [id, names[i]]))),
    [buffers, workspaceIds, names],
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
        const session = terminalSessionFields.get(buffer.sessionId)
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

      return displayNames.get(buffer.id) ?? buffer.name
    },
    [
      displayNames,
      getCommandLabel,
      getDirectoryLabel,
      isUsefulTerminalTitle,
      terminalSessionFields,
    ],
  )

  return getBufferDisplayName
}
