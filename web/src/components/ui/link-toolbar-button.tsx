// From the Plate registry (`https://platejs.org/r/floating-toolbar-kit.json`
// -> `link-toolbar-button`), verbatim — no primitive collision. Opens the
// same floating link editor already wired up by `LinkKit` (`link-toolbar.tsx`).
'use client'

import * as React from 'react'

import { useLinkToolbarButton, useLinkToolbarButtonState } from '@platejs/link/react'
import { Link } from 'lucide-react'

import { ToolbarButton } from './toolbar'

export function LinkToolbarButton(props: React.ComponentProps<typeof ToolbarButton>) {
  const state = useLinkToolbarButtonState()
  const { props: buttonProps } = useLinkToolbarButton(state)

  return (
    <ToolbarButton {...props} {...buttonProps} data-plate-focus tooltip="Link">
      <Link />
    </ToolbarButton>
  )
}
