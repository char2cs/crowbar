'use client'

import { PlateElement, type PlateElementProps } from 'platejs/react'

export function CommentTableElement({ children, ...props }: PlateElementProps) {
  return (
    <PlateElement
      {...props}
      as="table"
      className="my-2 w-full table-fixed border-collapse overflow-hidden rounded-md border border-border text-sm"
    >
      <tbody>{children}</tbody>
    </PlateElement>
  )
}

export function CommentTableRowElement(props: PlateElementProps) {
  return <PlateElement {...props} as="tr" className="border-border border-b last:border-b-0" />
}

export function CommentTableCellElement(props: PlateElementProps) {
  return (
    <PlateElement
      {...props}
      as="td"
      className="border-border border-r px-2 py-1 align-top last:border-r-0"
    />
  )
}

export function CommentTableCellHeaderElement(props: PlateElementProps) {
  return (
    <PlateElement
      {...props}
      as="th"
      className="border-border border-r bg-muted/40 px-2 py-1 text-left font-semibold align-top last:border-r-0"
    />
  )
}

export function CommentCodeBlockElement({ children, ...props }: PlateElementProps) {
  return (
    <PlateElement {...props} className="my-2">
      <pre className="overflow-x-auto rounded-md bg-muted/60 p-3 font-mono text-xs leading-relaxed [tab-size:2]">
        <code>{children}</code>
      </pre>
    </PlateElement>
  )
}

export function CommentCodeLineElement(props: PlateElementProps) {
  return <PlateElement {...props} />
}
