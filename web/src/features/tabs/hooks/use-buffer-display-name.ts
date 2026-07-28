import { useCallback, useMemo } from 'react'
import { useShallow } from 'zustand/react/shallow'
import { useTerminalStore } from '@/features/terminal/stores/terminal-store'
import type { PaneContent } from '@/features/panes/types/pane-content'
import { calculateDisplayNames } from '../utils/path-shortener'

interface UseBufferDisplayNameOptions {
  buffers: PaneContent[]
  rootFolderPath: string | undefined
}

// Separates the projected fields joined into each tuple string below. The
// brief this hook implements space-joins the fields, but terminal titles and
// cwd paths routinely contain spaces (e.g. "My Project", "/Users/x/My Docs"),
// which would let two different session states alias onto the same joined
// string and silently collapse the shallow-equality check. A control
// character that can never appear in a title or path is used instead.
const FIELD_SEP = ''

interface TerminalSessionFields {
  customName: boolean
  name: string
  title: string
  currentDirectory: string
}

/**
 * Returns `getBufferDisplayName(buffer)` — a stable callback that derives the
 * human-readable tab label from buffer metadata, terminal session state and
 * path-shortener disambiguation.
 */
export function useBufferDisplayName({ buffers, rootFolderPath }: UseBufferDisplayNameOptions) {
  // Subscribe only to the session fields this hook actually reads, projected
  // per terminal buffer into a flat string. Previously this subscribed to
  // the whole `sessions` Map, so every terminal's prompt redraw or `cd`
  // (which reallocates that Map) re-rendered every tab label in the app.
  // useShallow compares the resulting array element-by-element, so a change
  // to an unrelated session — or to a field of this session that no tab
  // label reads — never invalidates it.
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
