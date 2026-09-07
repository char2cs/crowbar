// Adapted from the Plate registry (`https://platejs.org/r/block-menu-kit.json`
// -> `block-context-menu`) — the per-block right-click menu.
//
// HAND-ADAPTED: this app's right-click menus are the OS's own native menu
// (see `@/components/ui/context-menu`'s `ContextMenu`/`useContextMenu`), not a
// React-rendered popup — a browser has to fake a menu; this app doesn't need
// to. Upstream renders through `@radix-ui/react-context-menu` directly; that
// import is gone here in favour of the shared `ContextMenu` API every other
// right-click menu in this app uses.
'use client'

import * as React from 'react'

import { BLOCK_CONTEXT_MENU_ID, BlockMenuPlugin, BlockSelectionPlugin } from '@platejs/selection/react'
import { KEYS } from 'platejs'
import { useEditorPlugin, useEditorReadOnly } from 'platejs/react'

import { useIsTouchDevice } from '@/hooks/use-is-touch-device'
import { setBlockType } from '@/components/editor/transforms'
import { ContextMenu, useContextMenu, type ContextMenuItem } from '@/components/ui/context-menu'

export function BlockContextMenu({ children }: { children: React.ReactNode }) {
  const { api, editor } = useEditorPlugin(BlockMenuPlugin)
  const isTouch = useIsTouchDevice()
  const readOnly = useEditorReadOnly()
  const menu = useContextMenu()
  const { openAt, close } = menu

  const handleTurnInto = React.useCallback(
    (type: string) => {
      editor
        .getApi(BlockSelectionPlugin)
        .blockSelection.getNodes()
        .forEach(([, path]) => {
          setBlockType(editor, type, { at: path })
        })
    },
    [editor],
  )

  const handleAlign = React.useCallback(
    (align: 'center' | 'left' | 'right') => {
      editor.getTransforms(BlockSelectionPlugin).blockSelection.setNodes({ align })
    },
    [editor],
  )

  // Closes both trackers together: this component's own popup state, and
  // Plate's `openId` — which `BlockSelectionPlugin` reads internally to decide
  // whether a block menu is currently open for a selected block.
  const handleClose = React.useCallback(() => {
    close()
    api.blockMenu.hide()
    editor.getApi(BlockSelectionPlugin).blockSelection.focus()
  }, [close, api, editor])

  if (isTouch) {
    return children
  }

  const items: ContextMenuItem[] = [
    {
      id: 'delete',
      label: 'Delete',
      onClick: () => {
        editor.getTransforms(BlockSelectionPlugin).blockSelection.removeNodes()
        editor.tf.focus()
      },
    },
    {
      id: 'duplicate',
      label: 'Duplicate',
      onClick: () => editor.getTransforms(BlockSelectionPlugin).blockSelection.duplicate(),
    },
    {
      id: 'turn-into',
      label: 'Turn into',
      onClick: () => {},
      items: [
        { id: 'turn-into-paragraph', label: 'Paragraph', onClick: () => handleTurnInto(KEYS.p) },
        { id: 'turn-into-h1', label: 'Heading 1', onClick: () => handleTurnInto(KEYS.h1) },
        { id: 'turn-into-h2', label: 'Heading 2', onClick: () => handleTurnInto(KEYS.h2) },
        { id: 'turn-into-h3', label: 'Heading 3', onClick: () => handleTurnInto(KEYS.h3) },
        { id: 'turn-into-blockquote', label: 'Blockquote', onClick: () => handleTurnInto(KEYS.blockquote) },
      ],
    },
    { id: 'sep-1', label: '', separator: true, onClick: () => {} },
    {
      id: 'indent',
      label: 'Indent',
      onClick: () => editor.getTransforms(BlockSelectionPlugin).blockSelection.setIndent(1),
    },
    {
      id: 'outdent',
      label: 'Outdent',
      onClick: () => editor.getTransforms(BlockSelectionPlugin).blockSelection.setIndent(-1),
    },
    {
      id: 'align',
      label: 'Align',
      onClick: () => {},
      items: [
        { id: 'align-left', label: 'Left', onClick: () => handleAlign('left') },
        { id: 'align-center', label: 'Center', onClick: () => handleAlign('center') },
        { id: 'align-right', label: 'Right', onClick: () => handleAlign('right') },
      ],
    },
  ]

  return (
    <div
      className="w-full"
      onContextMenu={(event) => {
        const dataset = (event.target as HTMLElement).dataset
        const disabled =
          dataset?.slateEditor === 'true' || readOnly || dataset?.plateOpenContextMenu === 'false'

        if (disabled) return event.preventDefault()

        event.preventDefault()
        const position = { x: event.clientX, y: event.clientY }
        setTimeout(() => {
          api.blockMenu.show(BLOCK_CONTEXT_MENU_ID, position)
          openAt(position)
        }, 0)
      }}
    >
      {children}
      <ContextMenu isOpen={menu.isOpen} position={menu.position} items={items} onClose={handleClose} />
    </div>
  )
}
