import type { TCodeSyntaxLeaf } from 'platejs'
import { type SlateLeafProps, SlateLeaf } from 'platejs/static'

export function CodeSyntaxLeafStatic(props: SlateLeafProps<TCodeSyntaxLeaf>) {
  return <SlateLeaf className={props.leaf.className as string} {...props} />
}
