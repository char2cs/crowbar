import { ElementApi, TextApi, type DecoratedRange } from 'platejs'
import type { PlateLeafProps } from 'platejs/react'
import { createPlatePlugin, PlateLeaf } from 'platejs/react'

const CHIP = 'chip'

// A command opens a word; an @path runs to the next space. Neither matches inside a path or URL.
const TOKENS: { kind: 'slash' | 'ref'; pattern: RegExp }[] = [
  { kind: 'slash', pattern: /(?<=^|\s)\/[A-Za-z][\w:-]*(?![\w/:-])(?!\.\w)/g },
  { kind: 'ref', pattern: /(?<=^|\s)@[\w./-]+/g },
]

function ChipLeaf(props: PlateLeafProps) {
  const kind = (props.leaf as unknown as Record<string, unknown>)[CHIP]
  return (
    <PlateLeaf {...props}>
      <span className={kind === 'slash' ? 'composer-slash' : 'composer-ref'} data-chip={`${kind}`}>
        {props.children}
      </span>
    </PlateLeaf>
  )
}

/** Marks, as decorations only, the commands and file references typed into the draft: the
 *  markdown the chat sends is untouched. A code block's text is code, so it is left alone. */
export const ChatComposerChipPlugin = createPlatePlugin({
  key: CHIP,
  node: { isLeaf: true },
  decorate: ({ editor, entry: [node, path] }) => {
    if (!TextApi.isText(node) || !node.text) return []
    const parent = editor.api.parent(path)?.[0]
    if (!parent || !ElementApi.isElement(parent) || parent.type !== 'p') return []
    return TOKENS.flatMap(({ kind, pattern }) =>
      [...node.text.matchAll(pattern)].map(
        (m) =>
          ({
            anchor: { path, offset: m.index },
            focus: { path, offset: m.index + m[0].length },
            [CHIP]: kind,
          }) as DecoratedRange,
      ),
    )
  },
}).withComponent(ChipLeaf)
