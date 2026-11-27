import { defaultRules, type MdRules } from '@platejs/markdown'

type Leaf = { text?: string; softBreak?: boolean }
type MdNode = { value?: unknown; children?: MdNode[] }

const SOFT = '\uE000'

function restoreSoftBreaks(node: MdNode): void {
  if (typeof node.value === 'string') node.value = node.value.replaceAll(SOFT, '\n')
  node.children?.forEach(restoreSoftBreaks)
}

const paragraph = defaultRules.p!

export const softBreakMarkdownRules: MdRules = {
  text: {
    deserialize(mdastNode, deco) {
      const value = (mdastNode as { value?: string }).value ?? ''
      const parts = value.split(/(\r?\n)/).filter((part) => part !== '')
      if (parts.length === 0) return { ...deco, text: '' }
      return parts.map(
        (part) =>
          part.endsWith('\n') ? { ...deco, text: part, softBreak: true } : { ...deco, text: part },
        // eslint-disable-next-line @typescript-eslint/no-explicit-any -- the lib types one leaf per text node, but a rule may return several
      ) as any
    },
  },
  p: {
    deserialize: paragraph.deserialize,
    serialize(node, options) {
      const children = (node.children as Leaf[]).map((child) =>
        child.softBreak && child.text
          ? { ...child, text: child.text.replace(/\r?\n/g, SOFT) }
          : child,
      )
      const md = paragraph.serialize!({ ...node, children } as typeof node, options)
      restoreSoftBreaks(md as MdNode)
      return md
    },
  },
}
