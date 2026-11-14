'use client'

import React from 'react'

import type { TListElement } from 'platejs'

import { isOrderedList } from '@platejs/list'
import type { PlateElementProps, RenderNodeWrapper } from 'platejs/react'

import { TodoLi, TodoMarker } from '@/components/ui/block-list-todo'

const config: Record<
  string,
  {
    Li: React.FC<PlateElementProps & { lineBreakBadge?: React.ReactNode }>
    Marker: React.FC<PlateElementProps>
  }
> = {
  todo: {
    Li: TodoLi,
    Marker: TodoMarker,
  },
}

export const BlockList: RenderNodeWrapper = (props) => {
  if (!props.element.listStyleType) return
  if (!isOrderedList(props.element)) return

  return (props) => <List {...props} />
}

function List(props: PlateElementProps & { lineBreakBadge?: React.ReactNode }) {
  const { listStart, listStyleType } = props.element as TListElement
  const { Li, Marker } = config[listStyleType] ?? {}
  const List = isOrderedList(props.element) ? 'ol' : 'ul'

  return (
    <List className="relative m-0 p-0" style={{ listStyleType }} start={listStart}>
      {Marker && <Marker {...props} />}
      {Li ? (
        <Li {...props} />
      ) : (
        <li>
          {props.children}
          {props.lineBreakBadge}
        </li>
      )}
    </List>
  )
}
