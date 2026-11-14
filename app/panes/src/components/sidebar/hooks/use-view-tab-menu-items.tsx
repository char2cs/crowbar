import { PushPinIcon, PushPinSlashIcon, XIcon } from '@phosphor-icons/react'
import { useStore } from 'zustand'
import type { ContextMenuItem } from '@/components/ui/context-menu'
import { closeRecent } from '@/components/sidebar/lib/recents-actions'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'

/** The entries only a tab in the view strip has: pin or unpin its view, and close it. */
export function useViewTabMenuItems(viewId: string | null): ContextMenuItem[] {
  const pinned = useStore(windowPaneStore, (s) => (viewId ? !!s.views[viewId]?.pinned : false))
  if (!viewId) return []
  const Icon = pinned ? PushPinSlashIcon : PushPinIcon
  return [
    {
      id: 'pin-tab',
      label: pinned ? 'Unpin Tab' : 'Pin Tab',
      icon: <Icon />,
      onClick: () => windowPaneStore.getState().paneActions.setViewPinned(viewId, !pinned),
    },
    { id: 'close-tab', label: 'Close', icon: <XIcon />, onClick: () => closeRecent(viewId) },
    { id: 'view-tab-separator', separator: true, label: '', onClick: () => {} },
  ]
}
