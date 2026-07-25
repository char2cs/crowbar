// Adapted from the Plate registry (`https://platejs.org/r/block-menu-kit.json`
// -> `block-context-menu`) — the per-block right-click menu.
//
// HAND-ADAPTED, twice over:
//
// 1. Upstream imports `ContextMenu`/`ContextMenuContent`/… from
//    `@/components/ui/context-menu` expecting Radix's declarative compound
//    API (`<ContextMenu><ContextMenuTrigger asChild>…`). This app's
//    `context-menu.tsx` is built on `@base-ui/react/context-menu` instead,
//    AND its `ContextMenu` export name is already claimed by a completely
//    different *imperative* API (`{ isOpen, position, items, onClose }`,
//    used by the tab bar / file explorer). Reusing that file would be
//    silently wrong (wrong props entirely), not just a style mismatch — so
//    per the task's guidance this file talks to `@radix-ui/react-context-menu`
//    directly instead, staying self-contained and never touching the shared
//    primitive.
// 2. Trimmed the "Ask AI" menu item (`@platejs/ai`, not installed) and the
//    "Turn into -> Code Drawing" entry (`@platejs/code-drawing`, not
//    installed — this app's mermaid support renders through the ordinary
//    code block's language switcher, not a dedicated node type).
'use client'

import * as React from 'react'

import {
  BLOCK_CONTEXT_MENU_ID,
  BlockMenuPlugin,
  BlockSelectionPlugin,
} from '@platejs/selection/react'
import * as ContextMenuPrimitive from '@radix-ui/react-context-menu'
import { ChevronRightIcon } from 'lucide-react'
import { KEYS } from 'platejs'
import { useEditorPlugin, useEditorReadOnly, usePluginOption } from 'platejs/react'

import { useIsTouchDevice } from '@/hooks/use-is-touch-device'
import { cn } from '@/lib/utils'
import { setBlockType } from '@/components/editor/transforms'

const menuContentClass =
  'z-50 min-w-[180px] overflow-hidden rounded-lg border border-border bg-popover p-1 text-popover-foreground shadow-md data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95'

const menuItemClass =
  'relative flex cursor-default items-center gap-1.5 rounded-md px-2 py-1.5 text-sm outline-none select-none focus:bg-accent focus:text-accent-foreground data-[disabled]:pointer-events-none data-[disabled]:opacity-50'

export function BlockContextMenu({ children }: { children: React.ReactNode }) {
  const { api, editor } = useEditorPlugin(BlockMenuPlugin)
  const isTouch = useIsTouchDevice()
  const readOnly = useEditorReadOnly()
  const openId = usePluginOption(BlockMenuPlugin, 'openId')
  const isOpen = openId === BLOCK_CONTEXT_MENU_ID

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

  if (isTouch) {
    return children
  }

  return (
    <ContextMenuPrimitive.Root
      onOpenChange={(open) => {
        if (!open) {
          api.blockMenu.hide()
        }
      }}
      modal={false}
    >
      <ContextMenuPrimitive.Trigger
        asChild
        onContextMenu={(event) => {
          const dataset = (event.target as HTMLElement).dataset
          const disabled =
            dataset?.slateEditor === 'true' || readOnly || dataset?.plateOpenContextMenu === 'false'

          if (disabled) return event.preventDefault()

          setTimeout(() => {
            api.blockMenu.show(BLOCK_CONTEXT_MENU_ID, {
              x: event.clientX,
              y: event.clientY,
            })
          }, 0)
        }}
      >
        <div className="w-full">{children}</div>
      </ContextMenuPrimitive.Trigger>
      {isOpen && (
        <ContextMenuPrimitive.Portal>
          <ContextMenuPrimitive.Content
            className={cn(menuContentClass, 'w-64')}
            onCloseAutoFocus={(e) => {
              e.preventDefault()
              editor.getApi(BlockSelectionPlugin).blockSelection.focus()
            }}
          >
            <ContextMenuPrimitive.Group>
              <ContextMenuPrimitive.Item
                className={menuItemClass}
                onClick={() => {
                  editor.getTransforms(BlockSelectionPlugin).blockSelection.removeNodes()
                  editor.tf.focus()
                }}
              >
                Delete
              </ContextMenuPrimitive.Item>
              <ContextMenuPrimitive.Item
                className={menuItemClass}
                onClick={() => {
                  editor.getTransforms(BlockSelectionPlugin).blockSelection.duplicate()
                }}
              >
                Duplicate
              </ContextMenuPrimitive.Item>
              <ContextMenuPrimitive.Sub>
                <ContextMenuPrimitive.SubTrigger className={cn(menuItemClass, 'justify-between')}>
                  Turn into
                  <ChevronRightIcon className="size-4" />
                </ContextMenuPrimitive.SubTrigger>
                <ContextMenuPrimitive.Portal>
                  <ContextMenuPrimitive.SubContent className={cn(menuContentClass, 'w-48')}>
                    <ContextMenuPrimitive.Item
                      className={menuItemClass}
                      onClick={() => handleTurnInto(KEYS.p)}
                    >
                      Paragraph
                    </ContextMenuPrimitive.Item>
                    <ContextMenuPrimitive.Item
                      className={menuItemClass}
                      onClick={() => handleTurnInto(KEYS.h1)}
                    >
                      Heading 1
                    </ContextMenuPrimitive.Item>
                    <ContextMenuPrimitive.Item
                      className={menuItemClass}
                      onClick={() => handleTurnInto(KEYS.h2)}
                    >
                      Heading 2
                    </ContextMenuPrimitive.Item>
                    <ContextMenuPrimitive.Item
                      className={menuItemClass}
                      onClick={() => handleTurnInto(KEYS.h3)}
                    >
                      Heading 3
                    </ContextMenuPrimitive.Item>
                    <ContextMenuPrimitive.Item
                      className={menuItemClass}
                      onClick={() => handleTurnInto(KEYS.blockquote)}
                    >
                      Blockquote
                    </ContextMenuPrimitive.Item>
                  </ContextMenuPrimitive.SubContent>
                </ContextMenuPrimitive.Portal>
              </ContextMenuPrimitive.Sub>
            </ContextMenuPrimitive.Group>

            <ContextMenuPrimitive.Separator className="-mx-1 my-1 h-px bg-border" />

            <ContextMenuPrimitive.Group>
              <ContextMenuPrimitive.Item
                className={menuItemClass}
                onClick={() =>
                  editor.getTransforms(BlockSelectionPlugin).blockSelection.setIndent(1)
                }
              >
                Indent
              </ContextMenuPrimitive.Item>
              <ContextMenuPrimitive.Item
                className={menuItemClass}
                onClick={() =>
                  editor.getTransforms(BlockSelectionPlugin).blockSelection.setIndent(-1)
                }
              >
                Outdent
              </ContextMenuPrimitive.Item>
              <ContextMenuPrimitive.Sub>
                <ContextMenuPrimitive.SubTrigger className={cn(menuItemClass, 'justify-between')}>
                  Align
                  <ChevronRightIcon className="size-4" />
                </ContextMenuPrimitive.SubTrigger>
                <ContextMenuPrimitive.Portal>
                  <ContextMenuPrimitive.SubContent className={cn(menuContentClass, 'w-48')}>
                    <ContextMenuPrimitive.Item
                      className={menuItemClass}
                      onClick={() => handleAlign('left')}
                    >
                      Left
                    </ContextMenuPrimitive.Item>
                    <ContextMenuPrimitive.Item
                      className={menuItemClass}
                      onClick={() => handleAlign('center')}
                    >
                      Center
                    </ContextMenuPrimitive.Item>
                    <ContextMenuPrimitive.Item
                      className={menuItemClass}
                      onClick={() => handleAlign('right')}
                    >
                      Right
                    </ContextMenuPrimitive.Item>
                  </ContextMenuPrimitive.SubContent>
                </ContextMenuPrimitive.Portal>
              </ContextMenuPrimitive.Sub>
            </ContextMenuPrimitive.Group>
          </ContextMenuPrimitive.Content>
        </ContextMenuPrimitive.Portal>
      )}
    </ContextMenuPrimitive.Root>
  )
}
