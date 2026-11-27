import { PathApi } from 'platejs'
import { createPlatePlugin } from 'platejs/react'

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
