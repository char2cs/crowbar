import { useEffect, useLayoutEffect, useRef } from 'react'
import type { KeyboardEvent, Ref } from 'react'
import { COMPOSER_LINE_HEIGHT } from '@/features/agent/composer/lib/handle-geometry'

interface ComposerFieldProps {
  value: string
  placeholder: string
  disabled?: boolean
  expanded: boolean
  controls?: string
  onChange: (value: string) => void
  onKeyDown: (event: KeyboardEvent<HTMLTextAreaElement>) => void
  /** Reported on every change, so the handle can ride the last line. */
  onHeightChange: (height: number) => void
  ref?: Ref<HTMLTextAreaElement>
}

/**
 * The composer's text field: one line that grows.
 *
 * The height is driven from `scrollHeight` rather than from a line count,
 * because a wrapped line and a typed newline are the same thing to a reader and
 * only the browser knows where the wrap fell. It is measured in a LAYOUT effect
 * so the handle moves in the same frame the box grows — a passive effect lets
 * the button lag the text by a frame, which reads as the button sliding.
 */
export function ComposerField({
  value,
  placeholder,
  disabled,
  expanded,
  controls,
  onChange,
  onKeyDown,
  onHeightChange,
  ref,
}: ComposerFieldProps) {
  const inner = useRef<HTMLTextAreaElement>(null)

  const attach = (node: HTMLTextAreaElement | null) => {
    inner.current = node
    if (typeof ref === 'function') ref(node)
    else if (ref) ref.current = node
  }

  useLayoutEffect(() => {
    const node = inner.current
    if (!node) return
    // Collapse before measuring: scrollHeight never shrinks against a height
    // that is already large enough to hold the old content.
    node.style.height = '0px'
    const next = Math.max(COMPOSER_LINE_HEIGHT, node.scrollHeight)
    node.style.height = `${next}px`
    onHeightChange(next)
  }, [value, onHeightChange])

  useEffect(() => {
    const node = inner.current
    if (!node || disabled) return
    const observer = new ResizeObserver(() => {
      node.style.height = '0px'
      const next = Math.max(COMPOSER_LINE_HEIGHT, node.scrollHeight)
      node.style.height = `${next}px`
      onHeightChange(next)
    })
    observer.observe(node)
    return () => observer.disconnect()
  }, [disabled, onHeightChange])

  return (
    <textarea
      ref={attach}
      className="field"
      rows={1}
      aria-label="Message the agent"
      aria-expanded={expanded}
      aria-controls={controls}
      value={value}
      placeholder={placeholder}
      disabled={disabled}
      onChange={(event) => onChange(event.target.value)}
      onKeyDown={onKeyDown}
    />
  )
}
