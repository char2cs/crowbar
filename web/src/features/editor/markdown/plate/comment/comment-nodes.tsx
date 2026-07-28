'use client'

import { PlateElement, type PlateElementProps } from 'platejs/react'

/**
 * Minimal node components for the comment editor's tables and code blocks.
 *
 * The app's full-editor equivalents (`@/components/ui/table-node`,
 * `@/components/ui/code-block-node`) are page-editor furniture: between them
 * they pull @platejs/dnd, @platejs/resizable, block selection, a Radix
 * popover, a cmdk language combobox, a colour dropdown, a table toolbar, and —
 * through CodeBlockKit — `createLowlight(all)`, i.e. every highlight.js
 * grammar. That is the right trade in a document you write for an hour. It is
 * the wrong trade in a box you type two sentences into, especially one that
 * lives on the branch-review surface, whose whole job is to stay fast on huge
 * diffs.
 *
 * These exist so the PLUGINS can still be registered. That matters more than
 * the affordances they drop: @platejs/markdown discards any node whose plugin
 * is absent, so an unregistered table plugin doesn't mean "comments have no
 * table toolbar", it means a comment containing a table LOSES it the moment
 * someone edits that comment. Round-tripping the content is the requirement;
 * resize handles are not.
 */

/** `table > tbody > tr > td` — the same DOM shape Plate's own TableElement
 *  builds, since Slate's children land directly under this element and a `<tr>`
 *  parented by `<table>` is invalid markup React will complain about. */
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

/**
 * A fenced block, unhighlighted.
 *
 * Highlighting is deliberately absent rather than merely unimplemented: the
 * POSTED comment renders through `MarkdownPreview`, which highlights with shiki
 * already, so paying for a second highlighter — lowlight, with every grammar —
 * to colour three lines mid-typing buys nothing the reader will ever see.
 */
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
