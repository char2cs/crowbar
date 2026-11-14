import type { TCodeSyntaxLeaf } from 'platejs'
import { type SlateLeafProps, SlateLeaf } from 'platejs/static'

/** A highlighted token in static rendering, which has no Plate store for `PlateLeaf`. */
export function CodeSyntaxLeafStatic(props: SlateLeafProps<TCodeSyntaxLeaf>) {
  return <SlateLeaf className={props.leaf.className as string} {...props} />
}
