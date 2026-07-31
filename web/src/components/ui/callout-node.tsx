'use client'

import * as React from 'react'

import { useCalloutEmojiPicker } from '@platejs/callout/react'
import { useEmojiDropdownMenuState } from '@platejs/emoji/react'
import { PlateElement } from 'platejs/react'

import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

import { EmojiPicker, EmojiPopover } from './emoji-toolbar-button'

/*
 * Oracle anchors (native/oracle/ANCHORS.md).
 *
 * **Neither declaration is made anywhere here.** Nothing is content-sized — the
 * callout takes the editor's measured column and both inner boxes are `w-full`
 * — and nothing is line-sized: the emoji control is the only anchor that paints
 * a run, and `size-6`/`sm:h-8` author its box at 24x32 around a 20px line box,
 * so declaring it would manufacture a 12px delta. That is `badge`'s rule, and
 * v1.6 states it as "derived from the line box", not "paints text".
 *
 * The emoji `<Button>` is renamed to `callout-emoji` rather than left as
 * `button`'s own id, per v1.8 and `git-row-badge`'s precedent.
 *
 * The `{children}` inside `callout-content` are Plate's own nodes — a
 * `.slate-p` belongs to `ParagraphElement`, not to this component — so they
 * carry no anchor from here.
 *
 * NOTE for anyone porting from this file: **every visual utility in the
 * `cn(...)` below is dead.** `.crowbar-markdown-editor .slate-callout` in
 * `features/editor/markdown/plate/markdown-editor.css` overrides `my-1`,
 * `rounded-sm`, `bg-muted`, `p-4` and `pl-3` outright. Measure the running
 * element; see `native/mapping/callout-node.md`.
 */

export function CalloutElement({
  attributes,
  children,
  className,
  ...props
}: React.ComponentProps<typeof PlateElement>) {
  const { emojiPickerState, isOpen, setIsOpen } = useEmojiDropdownMenuState({
    closeOnSelect: true,
  })

  const { emojiToolbarDropdownProps, props: calloutProps } = useCalloutEmojiPicker({
    isOpen,
    setIsOpen,
  })

  return (
    <PlateElement
      className={cn('my-1 flex rounded-sm bg-muted p-4 pl-3', className)}
      style={{
        backgroundColor: props.element.backgroundColor as React.CSSProperties['backgroundColor'],
      }}
      attributes={{
        ...attributes,
        'data-plate-open-context-menu': true,
        'data-oracle-id': 'callout',
      }}
      {...props}
    >
      <div className="flex w-full gap-2 rounded-md" data-oracle-id="callout-row">
        <EmojiPopover
          {...emojiToolbarDropdownProps}
          control={
            <Button
              variant="ghost"
              className="size-6 select-none p-1 text-[18px] hover:bg-muted-foreground/15"
              style={{
                fontFamily:
                  '"Apple Color Emoji", "Segoe UI Emoji", NotoColorEmoji, "Noto Color Emoji", "Segoe UI Symbol", "Android Emoji", EmojiSymbols',
              }}
              contentEditable={false}
              data-oracle-id="callout-emoji"
            >
              {(props.element.icon as string | undefined) || '💡'}
            </Button>
          }
        >
          <EmojiPicker {...emojiPickerState} {...calloutProps} />
        </EmojiPopover>
        <div className="w-full" data-oracle-id="callout-content">
          {children}
        </div>
      </div>
    </PlateElement>
  )
}
