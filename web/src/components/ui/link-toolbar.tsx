'use client'

import * as React from 'react'

import type { TLinkElement } from 'platejs'

import { type UseVirtualFloatingOptions, flip, offset } from '@platejs/floating'
import { getLinkAttributes } from '@platejs/link'
import {
  type LinkFloatingToolbarState,
  FloatingLinkUrlInput,
  useFloatingLinkEdit,
  useFloatingLinkEditState,
  useFloatingLinkInsert,
  useFloatingLinkInsertState,
} from '@platejs/link/react'
import { cva } from 'class-variance-authority'
import { ExternalLink, Link, Text, Unlink } from 'lucide-react'
import { KEYS } from 'platejs'
import { useEditorRef, useEditorSelection, useFormInputProps, usePluginOption } from 'platejs/react'

import { buttonVariants } from '@/components/ui/button-variants'
import { Separator } from '@/components/ui/separator'

const popoverVariants = cva(
  'z-50 w-auto rounded-md border bg-popover p-1 text-popover-foreground shadow-md outline-hidden',
)

const inputVariants = cva(
  'flex h-[28px] w-full rounded-md border-none bg-transparent px-1.5 py-1 text-base placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-transparent md:text-sm',
)

export function LinkFloatingToolbar({ state }: { state?: LinkFloatingToolbarState }) {
  const activeCommentId = usePluginOption({ key: KEYS.comment }, 'activeId')
  const activeSuggestionId = usePluginOption({ key: KEYS.suggestion }, 'activeId')

  const floatingOptions: UseVirtualFloatingOptions = React.useMemo(
    () => ({
      middleware: [
        offset(8),
        flip({
          fallbackPlacements: ['bottom-end', 'top-start', 'top-end'],
          padding: 12,
        }),
      ],
      placement: activeSuggestionId || activeCommentId ? 'top-start' : 'bottom-start',
    }),
    [activeCommentId, activeSuggestionId],
  )

  const insertState = useFloatingLinkInsertState({
    ...state,
    floatingOptions: {
      ...floatingOptions,
      ...state?.floatingOptions,
    },
  })
  const {
    hidden,
    props: insertProps,
    ref: insertRef,
    textInputProps,
  } = useFloatingLinkInsert(insertState)

  const editState = useFloatingLinkEditState({
    ...state,
    floatingOptions: {
      ...floatingOptions,
      ...state?.floatingOptions,
    },
  })
  const {
    editButtonProps,
    props: editProps,
    ref: editRef,
    unlinkButtonProps,
  } = useFloatingLinkEdit(editState)
  const inputProps = useFormInputProps({
    preventDefaultOnEnterKeydown: true,
  })

  if (hidden) return null

  const input = (
    <div className="flex w-[330px] flex-col" {...inputProps}>
      <div className="flex items-center">
        <div className="flex items-center pr-1 pl-2 text-muted-foreground">
          <Link className="size-4" />
        </div>

        <FloatingLinkUrlInput
          className={inputVariants()}
          placeholder="Paste link"
          data-plate-focus
        />
      </div>
      <Separator className="my-1" />
      <div className="flex items-center">
        <div className="flex items-center pr-1 pl-2 text-muted-foreground">
          <Text className="size-4" />
        </div>
        <input
          className={inputVariants()}
          placeholder="Text to display"
          data-plate-focus
          {...textInputProps}
        />
      </div>
    </div>
  )

  const editContent = editState.isEditing ? (
    input
  ) : (
    <div className="box-content flex items-center">
      <button
        className={buttonVariants({ size: 'sm', variant: 'ghost' })}
        type="button"
        {...editButtonProps}
      >
        Edit link
      </button>

      <Separator orientation="vertical" />

      <LinkOpenButton />

      <Separator orientation="vertical" />

      <button
        className={buttonVariants({
          size: 'sm',
          variant: 'ghost',
        })}
        type="button"
        {...unlinkButtonProps}
      >
        <Unlink width={18} />
      </button>
    </div>
  )

  return (
    <>
      <div ref={insertRef} className={popoverVariants()} {...insertProps}>
        {input}
      </div>

      <div ref={editRef} className={popoverVariants()} {...editProps}>
        {editContent}
      </div>
    </>
  )
}

function LinkOpenButton() {
  const editor = useEditorRef()
  const selection = useEditorSelection()

  const attributes = React.useMemo(
    () => {
      const entry = editor.api.node<TLinkElement>({
        match: { type: editor.getType(KEYS.link) },
      })
      if (!entry) {
        return {}
      }
      const [element] = entry
      return getLinkAttributes(editor, element)
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [editor, selection],
  )

  return (
    <a
      {...attributes}
      className={buttonVariants({
        size: 'sm',
        variant: 'ghost',
      })}
      // react-doctor-disable-next-line mouse-events-have-key-events -- this is not a feature handler, it is a bubbling guard: the floating link toolbar's parent listens for hover to decide whether to stay open, and this `<a>` swallows the mouseover so hovering the "open in new tab" affordance doesn't re-trigger it. A keyboard user loses nothing (there is no hover to miss), and the focus-event analogue the rule asks for would swallow `focusin` from the same parent, which the toolbar does rely on. The link itself is a real focusable `<a>` with an `aria-label`.
      onMouseOver={(e) => {
        e.stopPropagation()
      }}
      aria-label="Open link in a new tab"
      target="_blank"
    >
      <ExternalLink width={18} />
    </a>
  )
}
