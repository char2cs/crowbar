import { useEffect } from 'react'
import { CODE_LINE_TO_DECORATIONS, setCodeBlockToDecorations } from '@platejs/code-block'
import { type TCodeBlockElement, type TElement, NodeApi } from 'platejs'
import { type PlateEditor, usePlateStore } from 'platejs/react'
import { onGrammarReady, readyLanguageIds } from '@/features/editor/lib/shiki-grammars'

function decoratedPlain(element: TCodeBlockElement): boolean {
  const lines = element.children as TElement[]
  return (
    lines.some((line) => NodeApi.string(line) !== '') &&
    lines.every((line) => !CODE_LINE_TO_DECORATIONS.get(line)?.length)
  )
}

export function useRedecorateWhenGrammarReady(
  editor: PlateEditor,
  element: TCodeBlockElement,
): void {
  const plateStore = usePlateStore()
  useEffect(() => {
    const lang = element.lang
    if (!lang) return
    const redecorate = () => {
      const path = editor.api.findPath(element)
      if (!path) return
      setCodeBlockToDecorations(editor, [element, path])
      plateStore.set('versionDecorate', (plateStore.get('versionDecorate') ?? 0) + 1)
    }
    const off = onGrammarReady((ready) => {
      if (ready === lang) redecorate()
    })
    if (readyLanguageIds().includes(lang) && decoratedPlain(element)) redecorate()
    return off
  }, [editor, element, plateStore])
}
