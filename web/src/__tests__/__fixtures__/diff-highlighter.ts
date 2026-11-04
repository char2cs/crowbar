import { getSharedHighlighter } from '@pierre/diffs'

/** Compiles the TypeScript grammar the diff surface tokenises with (~0.7s of CPU on
 *  first use). Await it at collection, which has no test timeout, so that cost
 *  never lands inside whichever test renders a diff first. */
export async function warmDiffHighlighter(): Promise<void> {
  const themes = { dark: 'pierre-dark', light: 'pierre-light' }
  const highlighter = await getSharedHighlighter({
    themes: Object.values(themes),
    langs: ['typescript'],
  })
  highlighter.codeToTokens('export const x = 1', { lang: 'typescript', themes })
}
