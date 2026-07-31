import { Button } from '@/components/ui/button'

interface InlineErrorProps {
  error: Error
  onRetry: () => void
  title?: string
}

/*
 * Oracle anchors (native/oracle/ANCHORS.md).
 *
 * `data-oracle-content-sized` is on all four children and **not** on the panel:
 * `items-center` on a flex column leaves the cross axis unstretched, so each
 * child's width is its own content's, while the panel itself is `flex-1`.
 *
 * `data-oracle-line-sized` is on the three bare runs and **not** on the retry
 * control, which authors `h-8 sm:h-7` — v1.6's test is "derived from the line
 * box", not "paints text", and `badge` is the precedent. The detail line is
 * v1.6's exact case: `text-[11px]` resolves to a 16.5px line box that WebKit
 * floors into a 16px box.
 *
 * The retry `<Button>` is renamed to `inline-error-retry` rather than left as
 * `button`'s own id. v1.8: an anchor beneath the root that belongs to another
 * surface is not part of this one, and a call-site rename is what makes it
 * genuinely this component's — `git-row-badge` is the precedent.
 *
 * No anchor set is declared in `oracleSurfaceScope`: the detail line is behind
 * `import.meta.env.DEV`, so the set is a property of the *build* rather than of
 * the surface, and a declared anchor that is legitimately absent is a refusal.
 */
export function InlineError({ error, onRetry, title = 'Failed to load' }: InlineErrorProps) {
  return (
    <div
      className="flex flex-1 flex-col items-center justify-center gap-2 p-6 text-center"
      data-oracle-id="inline-error"
    >
      <span
        className="text-lg opacity-50"
        aria-hidden="true"
        data-oracle-content-sized="true"
        data-oracle-line-sized="true"
        data-oracle-id="inline-error-glyph"
      >
        ⚠
      </span>
      <p
        className="text-sm font-medium text-foreground"
        data-oracle-content-sized="true"
        data-oracle-line-sized="true"
        data-oracle-id="inline-error-title"
      >
        {title}
      </p>
      {import.meta.env.DEV && (
        <p
          className="font-mono text-[11px] text-muted-foreground"
          data-oracle-content-sized="true"
          data-oracle-line-sized="true"
          data-oracle-id="inline-error-detail"
        >
          {error.message}
        </p>
      )}
      <Button
        variant="outline"
        size="sm"
        onClick={onRetry}
        className="mt-1"
        data-oracle-content-sized="true"
        data-oracle-id="inline-error-retry"
      >
        ↺ Retry
      </Button>
    </div>
  )
}
