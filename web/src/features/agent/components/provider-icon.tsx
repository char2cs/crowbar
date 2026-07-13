import { cn } from '@/utils/cn'

interface ProviderIconProps {
  /** Raw SVG markup for the provider, as served by the backend. */
  svg: string
  className?: string
}

/**
 * Inline the backend-trusted SVG markup so its `fill="currentColor"` paths
 * inherit the ambient text-* theme token from whichever button/row/tab hosts it —
 * an <img src="..."> can't inherit currentColor or size to its container the
 * same way. Mirrors FlickerSpinner's approach (components/ui/flicker-spinner.tsx).
 */
export function ProviderIcon({ svg, className }: ProviderIconProps) {
  return (
    <span
      aria-hidden="true"
      data-provider-icon=""
      className={cn(
        'inline-flex size-3.5 shrink-0 items-center justify-center [&>svg]:size-full',
        className,
      )}
      dangerouslySetInnerHTML={{ __html: svg }}
    />
  )
}
