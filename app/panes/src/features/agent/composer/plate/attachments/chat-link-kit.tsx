'use client'

import { LinkRules } from '@platejs/link'
import { isUrl } from 'platejs'
import { LinkPlugin } from '@platejs/link/react'
import { LinkFloatingToolbar } from '@/components/ui/link-toolbar'
import { ChatLinkElement, ChatLinkElementStatic } from './chat-attachment-file-card'

type RuleContext = Parameters<NonNullable<Parameters<typeof LinkRules.autolink>[0]['enabled']>>[0]

const outsideCode = (context: RuleContext) =>
  !context.editor.api.marks()?.code &&
  (context.getBlockTextBeforeSelection().split('`').length - 1) % 2 === 0

const inputRules = [
  LinkRules.markdown(),
  LinkRules.autolink({ variant: 'paste', enabled: outsideCode }),
  LinkRules.autolink({ variant: 'space', enabled: outsideCode }),
  LinkRules.autolink({ variant: 'break', enabled: outsideCode }),
]

const TRAILING_PUNCTUATION = /[.,:;!?*_~'"]$/
const linksAsTyped = (text: string) => !TRAILING_PUNCTUATION.test(text) && isUrl(text)

export const ChatLinkKit = [
  LinkPlugin.configure({
    inputRules,
    options: { isUrl: linksAsTyped },
    render: { node: ChatLinkElement, afterEditable: () => <LinkFloatingToolbar /> },
  }),
]

export const ChatLinkKitStatic = [
  LinkPlugin.configure({ inputRules, render: { node: ChatLinkElementStatic } }),
]
