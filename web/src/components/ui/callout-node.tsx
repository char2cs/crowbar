'use client'

import * as React from 'react'

import { useCalloutEmojiPicker } from '@platejs/callout/react'
import { useEmojiDropdownMenuState } from '@platejs/emoji/react'
import { PlateElement } from 'platejs/react'

import { Button } from '@/components/ui/button'

import { CalloutBody, EMOJI_FONT_FAMILY, calloutClassName } from './callout-content'
import { EmojiPicker, EmojiPopover } from './emoji-toolbar-button'

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

  const icon = props.element.icon as string | undefined

  return (
    <PlateElement
      className={calloutClassName(className)}
      style={{
        backgroundColor: props.element.backgroundColor as React.CSSProperties['backgroundColor'],
      }}
      attributes={{
        ...attributes,
        'data-plate-open-context-menu': true,
      }}
      {...props}
    >
      <CalloutBody
        icon={icon}
        iconSlot={
          <EmojiPopover
            {...emojiToolbarDropdownProps}
            control={
              <Button
                variant="ghost"
                className="size-6 select-none p-1 text-[18px] hover:bg-muted-foreground/15"
                style={{ fontFamily: EMOJI_FONT_FAMILY }}
                contentEditable={false}
              >
                {icon || '💡'}
              </Button>
            }
          >
            <EmojiPicker {...emojiPickerState} {...calloutProps} />
          </EmojiPopover>
        }
      >
        {children}
      </CalloutBody>
    </PlateElement>
  )
}
