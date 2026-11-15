import { nanoid } from 'nanoid'
import type { LayoutNode, LayoutSplit } from '@/features/panes/types/pane'

/** A split with exactly the sizes given: normalising them is the layout API's job, not this builder's. */
export function createSplit(
  direction: 'horizontal' | 'vertical',
  first: LayoutNode,
  second: LayoutNode,
  sizes: [number, number] = [50, 50],
): LayoutSplit {
  return { type: 'split', id: nanoid(), direction, sizes, first, second }
}
