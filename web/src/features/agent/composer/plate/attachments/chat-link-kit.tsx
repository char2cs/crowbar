'use client'

import { LinkRules } from '@platejs/link'
import { isUrl } from 'platejs'
import { LinkPlugin } from '@platejs/link/react'
import { LinkFloatingToolbar } from '@/components/ui/link-toolbar'
import { ChatLinkElement, ChatLinkElementStatic } from './chat-attachment-file-card'

const inputRules = [
  LinkRules.markdown(),
  LinkRules.autolink({ variant: 'paste' }),
  LinkRules.autolink({ variant: 'space' }),
  LinkRules.autolink({ variant: 'break' }),
]

/** GFM ends an address before its trailing punctuation; a word that ends in
 *  one is left as text, so what is linked is what the transcript links. */
const TRAILING_PUNCTUATION = /[.,:;!?*_~'"]$/
const linksAsTyped = (text: string) => !TRAILING_PUNCTUATION.test(text) && isUrl(text)

/** The shared `LinkKit` with a node renderer that draws attachment file cards. */
export const ChatLinkKit = [
  LinkPlugin.configure({
    inputRules,
    options: { isUrl: linksAsTyped },
    render: { node: ChatLinkElement, afterEditable: () => <LinkFloatingToolbar /> },
  }),
]

// No toolbar: LinkFloatingToolbar calls useEditorRef(), valid only in an
// interactive editor. The static renderer needs no `DndScope` either.
export const ChatLinkKitStatic = [
  LinkPlugin.configure({ inputRules, render: { node: ChatLinkElementStatic } }),
]
