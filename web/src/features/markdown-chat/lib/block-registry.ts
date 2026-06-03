import type { ComponentType, LazyExoticComponent } from 'react'
import type { WidgetData } from '../types'
import type { BlockInfo } from './parse-block-info'

// The shared registry of embeddable block types (code, mermaid, excalidraw, …).
// Both surfaces consume it: the read-only history renders `View`, and the
// CodeMirror input mounts `Editor ?? View`. Adding a block type = one entry +
// one component, with no knowledge of CodeMirror required.

export type BlockStorage =
  // The fence body IS the data (code, mermaid, math). Stateless render.
  | 'inline'
  // The fence carries a `widget-id`; the payload lives in the turn's widgets[].
  | 'referenced'

export interface BlockViewProps {
  type: string
  params: Record<string, string>
  /** Fence body — the source text (for `inline` blocks). */
  source: string
  /** Referenced payload, looked up by widget-id (for `referenced` blocks). */
  widget?: WidgetData
  /** True while the enclosing turn is still streaming (block may be incomplete). */
  streaming?: boolean
  /** Present only for editable/referenced blocks mounted in the input. */
  onChange?: (payload: unknown) => void
}

type BlockComponent =
  | ComponentType<BlockViewProps>
  | LazyExoticComponent<ComponentType<BlockViewProps>>

export interface BlockExtension {
  type: string
  storage: BlockStorage
  /** Does this extension own the given fenced block? */
  match: (info: BlockInfo) => boolean
  /** Read-only render (history) and default render (input). */
  View: BlockComponent
  /** Optional editable render mounted in the CodeMirror input. */
  Editor?: BlockComponent
  /** `referenced` blocks: produce an empty payload when inserted. */
  createPayload?: () => unknown
}

const registry: BlockExtension[] = []

export function registerBlock(ext: BlockExtension): void {
  // Replace an existing entry of the same type so module re-evaluation (HMR)
  // doesn't accumulate duplicates.
  const existing = registry.findIndex((e) => e.type === ext.type)
  if (existing >= 0) registry[existing] = ext
  else registry.push(ext)
}

/** First extension whose `match` accepts the info string, if any. */
export function findBlock(info: BlockInfo): BlockExtension | undefined {
  return registry.find((e) => e.match(info))
}

export function getBlock(type: string): BlockExtension | undefined {
  return registry.find((e) => e.type === type)
}

export function allBlocks(): readonly BlockExtension[] {
  return registry
}
