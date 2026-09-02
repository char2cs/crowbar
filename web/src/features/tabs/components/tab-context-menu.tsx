import {
  Copy,
  FolderOpen,
  PencilSimpleLine,
  PushPin as Pin,
  PushPinSlash as PinOff,
  ArrowCounterClockwise as RotateCcw,
  TerminalWindow as Terminal,
  X,
} from '@phosphor-icons/react'
import type { PaneContent } from '@/features/panes/types/pane-content'
import { isVirtualContent } from '@/features/panes/types/pane-content'
import { useTerminalStore } from '@/features/terminal/stores/terminal-store'
import { stripControlChars } from '@/features/terminal/utils/control-chars'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { ContextMenu, type ContextMenuItem } from '@/components/ui/context-menu'
import { primitivePrompt } from '@/components/ui/primitive-dialog-service'
import { getBaseName, getDirName } from '@/utils/path-helpers'
import Keybinding from '@/components/ui/keybinding'
import { IS_MAC } from '@/utils/platform'

const closeKeys = [IS_MAC ? 'Cmd' : 'Ctrl', 'W']

interface TabContextMenuProps {
  isOpen: boolean
  position: { x: number; y: number }
  buffer: PaneContent | null
  paneId?: string
  onClose: () => void
  onPin: (bufferId: string) => void
  onCloseTab: (bufferId: string) => void
  onCloseOthers: (bufferId: string) => void
  onCloseAll: () => void
  onCloseToRight: (bufferId: string) => void
  onCopyPath?: (path: string) => void
  onCopyRelativePath?: (path: string) => void
  onReload?: (bufferId: string) => void
  onRevealInFinder?: (path: string) => void
  onSplitRight?: (paneId: string, bufferId: string) => void
  onSplitDown?: (paneId: string, bufferId: string) => void
}

const TabContextMenu = ({
  isOpen,
  position,
  buffer,
  onClose,
  onPin,
  onCloseTab,
  onCloseOthers,
  onCloseAll,
  onCloseToRight,
  onCopyPath,
  onCopyRelativePath,
  onReload,
  onRevealInFinder,
  // paneId/onSplitRight/onSplitDown are still accepted (tab-bar.tsx keeps
  // wiring them from splitEditorGroup) but deliberately unused: spec §7.3 —
  // "a pane group is a group of chats, never of tabs" (Law 3) — a tab's
  // context menu no longer offers a way to split a pane.
}: TabContextMenuProps) => {
  if (!isOpen || !buffer) return null

  const items: ContextMenuItem[] = [
    {
      id: 'pin',
      label: buffer.isPinned ? 'Unpin Tab' : 'Pin Tab',
      icon: buffer.isPinned ? <PinOff /> : <Pin />,
      onClick: () => onPin(buffer.id),
    },
    ...(buffer.type === 'terminal'
      ? [
          {
            id: 'rename-terminal',
            label: 'Rename',
            icon: <PencilSimpleLine />,
            onClick: async () => {
              const rawName = await primitivePrompt('Enter a terminal name:', {
                title: 'Rename Terminal',
                defaultValue: buffer.name,
                placeholder: 'Terminal name',
                confirmLabel: 'Rename',
              })
              // Session `name` feeds the tab-label projection in
              // useBufferDisplayName (fields joined with \x01), so a pasted
              // control byte must be stripped before it reaches the store —
              // same ingress policy as OSC7 cwd paths and OSC titles.
              const nextName = rawName ? stripControlChars(rawName).trim() : ''

              if (!nextName) return

              useTerminalStore.getState().updateSession(buffer.sessionId, {
                name: nextName,
                customName: true,
              })
              windowPaneStore.setState((state) => ({
                buffers: state.buffers.map((b) =>
                  b.id === buffer.id ? { ...b, name: nextName } : b,
                ),
              }))
            },
          },
        ]
      : []),
    { id: 'sep-1', label: '', separator: true, onClick: () => {} },
    {
      id: 'copy-path',
      label: 'Copy Path',
      icon: <Copy />,
      onClick: async () => {
        if (onCopyPath) {
          onCopyPath(buffer.path)
          return
        }

        try {
          await navigator.clipboard.writeText(buffer.path)
        } catch (error) {
          console.error('Failed to copy path:', error)
        }
      },
    },
    {
      id: 'copy-relative-path',
      label: 'Copy Relative Path',
      icon: <Copy />,
      onClick: () => onCopyRelativePath?.(buffer.path),
    },
    {
      id: 'reveal',
      label: 'Reveal in Finder',
      icon: <FolderOpen />,
      onClick: () => onRevealInFinder?.(buffer.path),
    },
    ...(!isVirtualContent(buffer) && !buffer.path.includes('://')
      ? [
          {
            id: 'terminal',
            label: 'Open in Terminal',
            icon: <Terminal />,
            onClick: () => {
              const dirPath = getDirName(buffer.path)
              const dirName = getBaseName(dirPath, 'terminal')
              windowPaneStore.getState().bufferActions.openContent({
                type: 'terminal',
                name: dirName,
                workingDirectory: dirPath,
                workspaceId: buffer.workspaceId,
              })
            },
          },
        ]
      : []),
    ...(buffer.path !== 'extensions://marketplace'
      ? [
          {
            id: 'reload',
            label: 'Reload',
            icon: <RotateCcw />,
            onClick: () => onReload?.(buffer.id),
          },
        ]
      : []),
    { id: 'sep-3', label: '', separator: true, onClick: () => {} },
    {
      id: 'close',
      label: 'Close',
      icon: <X />,
      keybinding: <Keybinding keys={closeKeys} className="opacity-60" />,
      onClick: () => onCloseTab(buffer.id),
    },
    {
      id: 'close-others',
      label: 'Close Others',
      onClick: () => onCloseOthers(buffer.id),
    },
    {
      id: 'close-right',
      label: 'Close to Right',
      onClick: () => onCloseToRight(buffer.id),
    },
    {
      id: 'close-all',
      label: 'Close All',
      onClick: onCloseAll,
    },
  ]

  return <ContextMenu isOpen={isOpen} position={position} items={items} onClose={onClose} />
}

export default TabContextMenu
