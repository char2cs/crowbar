import { useEffect } from 'react'
import { CODE_LINE_TO_DECORATIONS, setCodeBlockToDecorations } from '@platejs/code-block'
import { type TCodeBlockElement, type TElement, NodeApi } from 'platejs'
import { type PlateEditor, usePlateStore } from 'platejs/react'
import { onGrammarReady, readyLanguageIds } from '@/features/editor/lib/shiki-grammars'

/** Text but no decorations: the block was decorated before its grammar was ready. */
function decoratedPlain(element: TCodeBlockElement): boolean {
  const lines = element.children as TElement[]
  return (
    lines.some((line) => NodeApi.string(line) !== '') &&
    lines.every((line) => !CODE_LINE_TO_DECORATIONS.get(line)?.length)
  )
}

/**
 * Highlighting grammars load on demand (shiki-grammars.ts): a block decorated as plain text
 * before its language landed is re-decorated once it has. A version bump, not
 * `editor.api.redecorate`: Plate binds that after this effect on mount.
 */
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
      // Filled before the bump: Slate re-queries the lines before their block.
      setCodeBlockToDecorations(editor, [element, path])
      plateStore.set('versionDecorate', (plateStore.get('versionDecorate') ?? 0) + 1)
    }
    const off = onGrammarReady((ready) => {
      if (ready === lang) redecorate()
    })
    // A grammar that landed between the block's decoration and this subscription told no one.
    if (readyLanguageIds().includes(lang) && decoratedPlain(element)) redecorate()
    return off
  }, [editor, element, plateStore])
}
