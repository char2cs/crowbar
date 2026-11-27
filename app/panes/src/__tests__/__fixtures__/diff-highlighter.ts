import { getSharedHighlighter } from '@pierre/diffs'

export async function warmDiffHighlighter(): Promise<void> {
  const themes = { dark: 'pierre-dark', light: 'pierre-light' }
  const highlighter = await getSharedHighlighter({
    themes: Object.values(themes),
    langs: ['typescript'],
  })
  highlighter.codeToTokens('export const x = 1', { lang: 'typescript', themes })
}
