import { render, waitFor } from '@testing-library/react'
import { expect, it } from 'vitest'
import { createPlateEditor, Plate, PlateContent } from 'platejs/react'
import { CodeBlockKit } from '@/components/editor/plugins/code-block-kit'
import { onGrammarReady } from '@/features/editor/lib/shiki-grammars'

function grammarReady(languageId: string): Promise<void> {
  return new Promise((resolve) => {
    const off = onGrammarReady((ready) => {
      if (ready !== languageId) return
      off()
      resolve()
    })
  })
}

function codeBlockEditor(lang: string, code: string) {
  return createPlateEditor({
    plugins: CodeBlockKit,
    value: [
      { type: 'code_block', lang, children: [{ type: 'code_line', children: [{ text: code }] }] },
    ],
  })
}

it('highlights a block once its grammar loads', async () => {
  const ready = grammarReady('go')
  const editor = codeBlockEditor('go', 'func main() {}')
  const { container } = render(
    <Plate editor={editor}>
      <PlateContent />
    </Plate>,
  )
  await ready

  await waitFor(() => expect(container.querySelector('.hljs-keyword')).toHaveTextContent('func'))
}, 30_000)

it('highlights a block whose grammar landed after its plain decoration but before it mounted', async () => {
  const ready = grammarReady('sql')
  const editor = codeBlockEditor('sql', 'select 1; -- x')
  // Decorated while the grammar loads: plain, and the load starts here.
  editor.tf.normalize({ force: true })
  await ready

  const { container } = render(
    <Plate editor={editor}>
      <PlateContent />
    </Plate>,
  )

  await waitFor(() => expect(container.querySelector('.hljs-keyword')).toHaveTextContent('select'))
}, 30_000)
