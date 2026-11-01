'use client'

import { LinkRules } from '@platejs/link'
import { LinkPlugin } from '@platejs/link/react'
import { LinkFloatingToolbar } from '@/components/ui/link-toolbar'
import { ChatLinkElement, ChatLinkElementStatic } from './chat-attachment-file-card'

const inputRules = [
  LinkRules.markdown(),
  LinkRules.autolink({ variant: 'paste' }),
  LinkRules.autolink({ variant: 'space' }),
  LinkRules.autolink({ variant: 'break' }),
]

/** The shared `LinkKit` with a node renderer that draws attachment file cards. */
export const ChatLinkKit = [
  LinkPlugin.configure({
    inputRules,
    render: { node: ChatLinkElement, afterEditable: () => <LinkFloatingToolbar /> },
  }),
]

// No toolbar: LinkFloatingToolbar calls useEditorRef(), valid only in an
// interactive editor. The static renderer needs no `DndScope` either.
export const ChatLinkKitStatic = [
  LinkPlugin.configure({ inputRules, render: { node: ChatLinkElementStatic } }),
]
