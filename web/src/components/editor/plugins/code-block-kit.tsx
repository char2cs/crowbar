'use client'

import { type CodeBlockConfig, CodeBlockRules } from '@platejs/code-block'
import { CodeBlockPlugin, CodeLinePlugin, CodeSyntaxPlugin } from '@platejs/code-block/react'

import { CodeBlockElement, CodeLineElement, CodeSyntaxLeaf } from '@/components/ui/code-block-node'
import { shikiLowlight } from './shiki-lowlight'

export const CodeBlockKit = [
  CodeBlockPlugin.configure({
    inputRules: [CodeBlockRules.markdown({ on: 'match' })],
    node: { component: CodeBlockElement },
    // Plate types the option as a lowlight instance; shikiLowlight implements
    // the three methods it calls (highlight, highlightAuto, listLanguages).
    options: { lowlight: shikiLowlight as unknown as CodeBlockConfig['options']['lowlight'] },
    shortcuts: { toggle: { keys: 'mod+alt+8' } },
  }),
  CodeLinePlugin.withComponent(CodeLineElement),
  CodeSyntaxPlugin.withComponent(CodeSyntaxLeaf),
]
