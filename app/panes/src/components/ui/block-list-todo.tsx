'use client'

import type { ReactNode } from 'react'

import { type PlateElementProps, useReadOnly } from 'platejs/react'
import { useTodoListElement, useTodoListElementState } from '@platejs/list/react'

import { Checkbox } from '@/components/ui/checkbox'
import { cn } from '@/lib/utils'

// The todo-list renderers for `block-list.tsx`'s `config` lookup. They live in
// their own module so neither file declares more than two components.

export function TodoMarker(props: PlateElementProps) {
  const state = useTodoListElementState({ element: props.element })
  const { checkboxProps } = useTodoListElement(state)
  const readOnly = useReadOnly()

  return (
    <div contentEditable={false}>
      <Checkbox
        className={cn('-left-6 absolute top-1', readOnly && 'pointer-events-none')}
        {...checkboxProps}
      />
    </div>
  )
}

export function TodoLi(props: PlateElementProps & { lineBreakBadge?: ReactNode }) {
  return (
    <li
      className={cn(
        'list-none',
        (props.element.checked as boolean) && 'text-muted-foreground line-through',
      )}
    >
      {props.children}
      {props.lineBreakBadge}
    </li>
  )
}
