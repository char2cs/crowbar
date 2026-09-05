'use client'

import { LinkRules } from '@platejs/link'
import { LinkPlugin } from '@platejs/link/react'
import { LinkFloatingToolbar } from '@/components/ui/link-toolbar'
import { ChatLinkElement } from './chat-attachment-file-card'

const inputRules = [
  LinkRules.markdown(),
  LinkRules.autolink({ variant: 'paste' }),
  LinkRules.autolink({ variant: 'space' }),
  LinkRules.autolink({ variant: 'break' }),
]

/** Same `LinkRules`/toolbar as the shared `LinkKit`
 *  (components/editor/plugins/link-kit.tsx) — only the node renderer
 *  differs: `ChatLinkElement` renders a file card for a chat-attachment
 *  ref, falling through to the ordinary `LinkElement` for everything else. */
export const ChatLinkKit = [
  LinkPlugin.configure({
    inputRules,
    render: { node: ChatLinkElement, afterEditable: () => <LinkFloatingToolbar /> },
  }),
]

// Same ChatLinkElement, same LinkRules as ChatLinkKit — only render.afterEditable
// (the toolbar) is dropped, same reason LinkKitStatic drops it: LinkFloatingToolbar
// calls useEditorRef() unconditionally, which is only valid inside an interactive
// editor.
export const ChatLinkKitStatic = [
  LinkPlugin.configure({ inputRules, render: { node: ChatLinkElement } }),
]
