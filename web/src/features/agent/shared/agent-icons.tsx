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

export const PlusIcon = icon(<path d="M12 5.5v13M5.5 12h13" />, 'PlusIcon')

/* The classic dog-eared page: the fold at top-right is what reads as "a
   file", distinguishing it from ChatIcon's speech-bubble outline. */
export const FileIcon = icon(
  <>
    <path d="M7 3.5h7l4.5 4.5v12.5H7z" />
    <path d="M14 3.5v4.5h4.5" />
  </>,
  'FileIcon',
)

export const SearchIcon = icon(
  <>
    <circle cx="10.5" cy="10.5" r="6" />
    <path d="m15 15 4.5 4.5" />
  </>,
  'SearchIcon',
)

export const FetchIcon = icon(
  <>
    <path d="M12 4.5v10" />
    <path d="m8 11 4 4 4-4" />
    <path d="M5 19.5h14" />
  </>,
  'FetchIcon',
)

export const ToolIcon = icon(
  <>
    <path d="M14.5 6.5a4 4 0 0 0-5 5L4.5 16.5a2.1 2.1 0 0 0 3 3l5-5a4 4 0 0 0 5-5l-2.5 2.5-3-3z" />
  </>,
  'ToolIcon',
)

export const AlertIcon = icon(
  <>
    <circle cx="12" cy="12" r="8.5" />
    <path d="M12 7.5v5.5M12 16.5h.01" />
  </>,
  'AlertIcon',
)

export const GaugeIcon = icon(
  <>
    <path d="M4.5 16.5a8 8 0 1 1 15 0" />
    <path d="m12 13.5 4-4M7.5 17.5h9" />
  </>,
  'GaugeIcon',
)

export const LinkIcon = icon(
  <>
    <path d="m9.5 14.5 5-5" />
    <path d="M7.5 16.5 6 18a2.8 2.8 0 0 1-4-4l3-3a2.8 2.8 0 0 1 4 0" />
    <path d="m16.5 7.5 1.5-1.5a2.8 2.8 0 0 1 4 4l-3 3a2.8 2.8 0 0 1-4 0" />
  </>,
  'LinkIcon',
)

export const ListIcon = icon(
  <>
    <path d="m4.5 7 1.2 1.2L8 5.8M10.5 7h9" />
    <path d="m4.5 12 1.2 1.2L8 10.8M10.5 12h9" />
    <path d="m4.5 17 1.2 1.2L8 15.8M10.5 17h9" />
  </>,
  'ListIcon',
)

export const RefreshIcon = icon(
  <>
    <path d="M19 8a7.5 7.5 0 0 0-12.8-2L4.5 8" />
    <path d="M4.5 4.5V8H8" />
    <path d="M5 16a7.5 7.5 0 0 0 12.8 2l1.7-2" />
    <path d="M19.5 19.5V16H16" />
  </>,
  'RefreshIcon',
)

export const OfflineIcon = icon(
  <>
    <path d="M5 10a11 11 0 0 1 4-2M15 8a11 11 0 0 1 4 2M8 14a6 6 0 0 1 8 0M11 18h2" />
    <path d="m4 4 16 16" />
  </>,
  'OfflineIcon',
)
