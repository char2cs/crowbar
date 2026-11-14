import { PathApi } from 'platejs'
import { createPlatePlugin } from 'platejs/react'

/** A lookup of a path the document lacks answers undefined without asking Slate, whose throw
 *  serialises the whole document into its message; the list plugin probes the path after
 *  every inserted node, so a streamed answer paid that per line. */
export const ChatMissingPathPlugin = createPlatePlugin({ key: 'chat-missing-path' }).overrideEditor(
  ({ editor, api: { node } }) => ({
    api: {
      node: ((at, options) => {
        if (PathApi.isPath(at) && !editor.api.hasPath(at)) return undefined
        return node(at, options)
      }) as typeof node,
    },
  }),
)
