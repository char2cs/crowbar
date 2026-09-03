'use client'

import { LinkRules } from '@platejs/link'
import { LinkPlugin } from '@platejs/link/react'

import { LinkElement } from '@/components/ui/link-node'
import { LinkFloatingToolbar } from '@/components/ui/link-toolbar'

export const LinkKit = [
  LinkPlugin.configure({
    inputRules: [
      LinkRules.markdown(),
      LinkRules.autolink({ variant: 'paste' }),
      LinkRules.autolink({ variant: 'space' }),
      LinkRules.autolink({ variant: 'break' }),
    ],
    render: {
      node: LinkElement,
      afterEditable: () => <LinkFloatingToolbar />,
    },
  }),
]

// Same LinkElement, same LinkRules as LinkKit — only render.afterEditable (the
// toolbar) is dropped: LinkFloatingToolbar calls useEditorRef() unconditionally,
// which is only valid inside an interactive editor.
export const LinkKitStatic = [
  LinkPlugin.configure({
    inputRules: [
      LinkRules.markdown(),
      LinkRules.autolink({ variant: 'paste' }),
      LinkRules.autolink({ variant: 'space' }),
      LinkRules.autolink({ variant: 'break' }),
    ],
    render: { node: LinkElement },
  }),
]
