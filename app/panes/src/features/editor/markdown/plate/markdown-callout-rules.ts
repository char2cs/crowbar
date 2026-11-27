import type { MdRootContent, MdRules } from '@platejs/markdown'
import { convertNodesDeserialize, convertNodesSerialize, defaultRules } from '@platejs/markdown'
import { KEYS, getPluginType } from 'platejs'
import type { TCalloutElement } from 'platejs'

const ALERT_MARKER = /^\[!([A-Za-z][\w-]*)\](?:\n|$)/

interface MdTextNode {
  type: 'text'
  value: string
  [key: string]: unknown
}

interface MdParagraphNode {
  type: 'paragraph'
  children: unknown[]
  [key: string]: unknown
}

function isParagraphNode(node: unknown): node is MdParagraphNode {
  return (
    typeof node === 'object' &&
    node !== null &&
    (node as { type?: unknown }).type === 'paragraph' &&
    Array.isArray((node as { children?: unknown }).children)
  )
}

function isTextNode(node: unknown): node is MdTextNode {
  return (
    typeof node === 'object' &&
    node !== null &&
    (node as { type?: unknown }).type === 'text' &&
    typeof (node as { value?: unknown }).value === 'string'
  )
}

function extractAlert(children: unknown[]): { variant: string; rest: unknown[] } | undefined {
  const [first, ...remainder] = children
  if (!isParagraphNode(first)) return undefined
  const [firstText, ...restOfFirstParagraph] = first.children
  if (!isTextNode(firstText)) return undefined
  const match = ALERT_MARKER.exec(firstText.value)
  if (!match) return undefined

  const variant = match[1].toLowerCase()
  const strippedValue = firstText.value.slice(match[0].length)
  const rest: unknown[] = []
  if (strippedValue.length > 0 || restOfFirstParagraph.length > 0) {
    rest.push({
      ...first,
      children:
        strippedValue.length > 0
          ? [{ ...firstText, value: strippedValue }, ...restOfFirstParagraph]
          : restOfFirstParagraph,
    })
  }
  rest.push(...remainder)
  return { variant, rest }
}

export const calloutMarkdownRules: MdRules = {
  blockquote: {
    deserialize(mdastNode, deco, options) {
      const alert = extractAlert((mdastNode as { children: unknown[] }).children)
      if (!alert) {
        return defaultRules.blockquote!.deserialize!(mdastNode, deco, options)
      }
      return {
        type: getPluginType(options.editor!, KEYS.callout),
        variant: alert.variant,
        children: convertNodesDeserialize(alert.rest as MdRootContent[], deco, options),
      } as unknown as TCalloutElement
    },
  },
  callout: {
    serialize(node, options) {
      const calloutNode = node as TCalloutElement
      const variant =
        typeof calloutNode.variant === 'string' && calloutNode.variant.trim().length > 0
          ? calloutNode.variant.trim()
          : 'note'
      const markerText = `[!${variant.toUpperCase()}]`
      const bodyChildren = convertNodesSerialize(calloutNode.children, options)
      const [firstBody, ...restBody] = bodyChildren

      const children =
        firstBody && isParagraphNode(firstBody)
          ? [
              {
                ...firstBody,
                children: [{ type: 'text', value: `${markerText}\n` }, ...firstBody.children],
              },
              ...restBody,
            ]
          : [
              { type: 'paragraph', children: [{ type: 'text', value: markerText }] },
              ...bodyChildren,
            ]

      return { type: 'blockquote', children } as unknown as ReturnType<
        NonNullable<NonNullable<MdRules['callout']>['serialize']>
      >
    },
  },
}
