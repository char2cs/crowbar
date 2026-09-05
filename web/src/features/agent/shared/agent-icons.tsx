import type { SVGProps } from 'react'
import { cn } from '@/lib/utils'

/**
 * The chat surface's icon set, drawn from the design canvas.
 *
 * These are NOT interchangeable with lucide or phosphor. The canvas draws every
 * glyph on a 24-unit grid at `stroke-width: 1.6` with round caps and joins, and
 * a general-purpose set at its own weight (lucide ships 2, phosphor's regular is
 * heavier still) reads as a different family the moment it sits beside one of
 * these — thicker, with squarer terminals, on a different optical size. Mixing
 * them is what made the surface switcher look hand-drawn.
 *
 * Size is the only thing a caller chooses, and it chooses from the canvas's three
 * steps: 12 for a chip's affordance, 14 for the default, 16 for a control the
 * hand aims at.
 */
export type AgentIconSize = 12 | 14 | 16

interface AgentIconProps extends Omit<SVGProps<SVGSVGElement>, 'children'> {
  size?: AgentIconSize
}

function icon(path: React.ReactNode, displayName: string) {
  function AgentIcon({ size = 14, className, ...rest }: AgentIconProps) {
    return (
      <svg
        viewBox="0 0 24 24"
        width={size}
        height={size}
        fill="none"
        stroke="currentColor"
        strokeWidth={1.6}
        strokeLinecap="round"
        strokeLinejoin="round"
        aria-hidden="true"
        focusable="false"
        className={cn('shrink-0', className)}
        {...rest}
      >
        {path}
      </svg>
    )
  }
  AgentIcon.displayName = displayName
  return AgentIcon
}

export const ChatIcon = icon(
  <path d="M6 4.5h12A1.5 1.5 0 0 1 19.5 6v8a1.5 1.5 0 0 1-1.5 1.5h-7.2L6.5 19.5V15.5H6A1.5 1.5 0 0 1 4.5 14V6A1.5 1.5 0 0 1 6 4.5Z" />,
  'ChatIcon',
)

export const TerminalIcon = icon(
  <>
    <path d="M4 5h16v14H4z" />
    <path d="M8 10l2.5 2L8 14" />
    <path d="M13 15h4" />
  </>,
  'TerminalIcon',
)

/* Two panes side by side. The canvas switcher has no third segment — Split is a
   development instrument Crowbar adds — so this is drawn to the same grid and
   weight as its two neighbours rather than borrowed from another set. */
export const SplitIcon = icon(
  <>
    <path d="M4 5h16v14H4z" />
    <path d="M12 5v14" />
  </>,
  'SplitIcon',
)

export const UpIcon = icon(
  <>
    <path d="M12 19V6" />
    <path d="M6.5 11.5 12 6l5.5 5.5" />
  </>,
  'UpIcon',
)

export const UpDownIcon = icon(
  <>
    <path d="M8 9.5 12 5.5l4 4" />
    <path d="M16 14.5 12 18.5l-4-4" />
  </>,
  'UpDownIcon',
)

export const CheckIcon = icon(<path d="m5 12.5 4.5 4.5L19 7.5" />, 'CheckIcon')

export const CloseIcon = icon(<path d="m6 6 12 12M18 6 6 18" />, 'CloseIcon')

export const PencilIcon = icon(
  <path d="M16.5 4.5a2.1 2.1 0 0 1 3 3L8 19l-4 1 1-4z" />,
  'PencilIcon',
)

/* Filled, unlike every other glyph here: a stop is the one control whose meaning
   is "solid", and the canvas draws it as a rounded square rather than an outline. */
export const StopIcon = icon(
  <rect x="7" y="7" width="10" height="10" rx="1.6" fill="currentColor" stroke="none" />,
  'StopIcon',
)

export const SubagentIcon = icon(
  <>
    <circle cx="12" cy="5.5" r="2.4" />
    <circle cx="5.5" cy="18" r="2.4" />
    <circle cx="18.5" cy="18" r="2.4" />
    <path d="M10.3 7.4 6.9 15.7" />
    <path d="M13.7 7.4l3.4 8.3" />
  </>,
  'SubagentIcon',
)

export const CopyIcon = icon(
  <>
    <rect x="8.5" y="8.5" width="11" height="11" rx="2" />
    <path d="M15.5 8.5V6.5a2 2 0 0 0-2-2h-7a2 2 0 0 0-2 2v7a2 2 0 0 0 2 2h2" />
  </>,
  'CopyIcon',
)
