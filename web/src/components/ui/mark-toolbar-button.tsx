// From the Plate registry (`https://platejs.org/r/floating-toolbar-kit.json`
// -> `mark-toolbar-button`), verbatim — `./toolbar` resolves to this app's
// own `toolbar.tsx`, which is already Radix-toolbar-based and exports the
// same `ToolbarButton` shape this expects, so there is nothing to adapt.
'use client'

import * as React from 'react'

import { useMarkToolbarButton, useMarkToolbarButtonState } from 'platejs/react'

import { ToolbarButton } from './toolbar'

export function MarkToolbarButton({
  clear,
  nodeType,
  ...props
}: React.ComponentProps<typeof ToolbarButton> & {
  nodeType: string
  clear?: string[] | string
}) {
  const state = useMarkToolbarButtonState({ clear, nodeType })
  const { props: buttonProps } = useMarkToolbarButton(state)

  return <ToolbarButton {...props} {...buttonProps} />
}
