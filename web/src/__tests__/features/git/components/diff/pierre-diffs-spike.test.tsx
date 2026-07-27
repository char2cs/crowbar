import { render, waitFor } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { PatchDiff } from '@pierre/diffs/react'

/**
 * SPIKE — de-risks Phase 3 of
 * docs/superpowers/specs/2026-07-27-diff-subsystem-at-scale-design.md before any
 * of the diff stack is rewritten against @pierre/diffs.
 *
 * Four assumptions the phase rests on, none safe to take on faith:
 *   1. the library renders real diff content under React 19 + jsdom;
 *   2. `renderAnnotation` output lands in LIGHT DOM — it is portalled with a
 *      `slot` attribute — so review threads keep Tailwind and CSS-var tokens.
 *      Inside the shadow root the entire thread UI would render unstyled;
 *   3. the file header carries name and ± counts, which the review pane needs;
 *   4. a controlled line selection is accepted, since thread creation uses it.
 *
 * If this file fails, stop and re-plan Phase 3 — these are its load-bearing
 * assumptions, not incidental details.
 *
 * The first version of this spike asserted only `container.querySelector('*')`
 * and would have passed against almost anything. It also waited merely for
 * non-empty shadow text — which the file header satisfies immediately, before
 * a single code row exists — so its content assertions raced. Both are fixed
 * below: the wait blocks on a CODE marker, and the assertions check content.
 * A spike that cannot fail de-risks nothing.
 */

const PATCH = `diff --git a/src/greet.ts b/src/greet.ts
index 1111111..2222222 100644
--- a/src/greet.ts
+++ b/src/greet.ts
@@ -1,3 +1,3 @@
 export function greet(name: string) {
-  return "hi " + name
+  return \`hello \${name}\`
 }
`

interface ThreadPayload {
  label: string
}

/**
 * The rendered rows live in the custom element's shadow root, and they arrive
 * AFTER the header does.
 *
 * Waiting merely for non-empty text is the trap: the header ("src/greet.ts",
 * "-1", "+1") is present almost immediately, so such a wait returns before a
 * single code row exists and every content assertion then races. Worse, enough
 * of the header survives even when the component throws mid-construction that
 * header-only assertions pass against a broken render. Waiting for a CODE
 * marker is what makes these tests depend on the thing they claim to test.
 */
async function shadowTextOf(container: HTMLElement): Promise<string> {
  let text = ''
  await waitFor(() => {
    const host = container.querySelector('diffs-container')
    expect(host, 'expected a <diffs-container> host element').toBeTruthy()
    text = (host as HTMLElement).shadowRoot?.textContent ?? ''
    expect(text, 'code rows have not rendered yet').toContain('export function greet')
  })
  return text
}

describe('@pierre/diffs spike', () => {
  it('renders both sides of a unified patch', async () => {
    const { container } = render(<PatchDiff patch={PATCH} />)
    const text = await shadowTextOf(container)

    // Content, not merely a node: the removed line, the added line, and the
    // untouched context line all reach the DOM.
    expect(text).toContain('return "hi " + name')
    expect(text).toContain('return `hello ${name}`')
    expect(text).toContain('export function greet(name: string) {')
  })

  it('renders a file header with the path and ± counts', async () => {
    const { container } = render(<PatchDiff patch={PATCH} />)
    const text = await shadowTextOf(container)

    expect(text).toContain('src/greet.ts')
    expect(text).toContain('-1')
    expect(text).toContain('+1')
  })

  it('portals annotations into LIGHT dom, so app styling still applies', async () => {
    const { container } = render(
      <PatchDiff<ThreadPayload>
        patch={PATCH}
        lineAnnotations={[{ side: 'additions', lineNumber: 2, metadata: { label: 'thread-here' } }]}
        renderAnnotation={(a) => (
          <div data-testid="thread" className="rounded bg-background text-foreground">
            {a.metadata.label}
          </div>
        )}
      />,
    )

    // Reachable by a plain container query — i.e. in the light DOM, where the
    // document stylesheet applies. A shadow-only node would not be found here.
    let node: Element | null = null
    await waitFor(() => {
      node = container.querySelector('[data-testid="thread"]')
      expect(node).toBeTruthy()
    })

    // The annotation must sit beside a REAL rendered diff, not a broken one.
    await shadowTextOf(container)

    expect(node!.textContent).toBe('thread-here')
    // Tailwind classes survive verbatim — nothing rewrote or scoped them.
    expect(node!.className).toContain('bg-background')
    // And the wrapper carries the slot that projects it into the shadow tree.
    expect(node!.parentElement?.getAttribute('slot')).toBeTruthy()
  })

  it('accepts a controlled line selection, which thread creation is built on', async () => {
    const { container } = render(
      <PatchDiff patch={PATCH} selectedLines={{ start: 2, end: 2, side: 'additions' }} />,
    )
    const text = await shadowTextOf(container)
    expect(text).toContain('return `hello ${name}`')
  })
})
