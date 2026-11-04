'use client'

import type { SVGProps } from 'react'

type TableIconProps = SVGProps<SVGSVGElement>
type Edge = 'top' | 'right' | 'bottom' | 'left'

const GRID = [1, 3, 5, 7, 9, 11, 13]
// A 2×2 table's outline and centre cross, dotted; a solid edge replaces its dots.
const FRAME = new Set([1, 7, 13])
const ON_EDGE: Record<Edge, (x: number, y: number) => boolean> = {
  top: (_x, y) => y === 1,
  right: (x) => x === 13,
  bottom: (_x, y) => y === 13,
  left: (x) => x === 1,
}
const EDGE_BAR: Record<Edge, string> = {
  top: 'M14 1.75L1 1.75L1 0.25L14 0.25L14 1.75Z',
  right: 'M13.25 1L13.25 14L14.75 14L14.75 1L13.25 1Z',
  bottom: 'M1 13.25L14 13.25V14.75L1 14.75V13.25Z',
  left: 'M1.75 1L1.75 14L0.25 14L0.25 1L1.75 1Z',
}
const ALL_EDGES: Edge[] = ['top', 'right', 'bottom', 'left']
const OUTLINE =
  'M0.25 1C0.25 0.59 0.59 0.25 1 0.25H14C14.41 0.25 14.75 0.59 14.75 1V14C14.75 14.41 14.41 14.75 14 14.75H1C0.59 14.75 0.25 14.41 0.25 14V1ZM1.75 1.75V13.25H13.25V1.75H1.75Z'

function BorderIcon({ title, edges, ...props }: TableIconProps & { edges: Edge[]; title: string }) {
  const dots = GRID.flatMap((x) =>
    GRID.filter(
      (y) => (FRAME.has(x) || FRAME.has(y)) && !edges.some((edge) => ON_EDGE[edge](x, y)),
    ).map((y) => [x, y] as const),
  )
  const bars = edges.length === ALL_EDGES.length ? [OUTLINE] : edges.map((edge) => EDGE_BAR[edge])
  return (
    <svg
      fill="none"
      height="15"
      viewBox="0 0 15 15"
      width="15"
      xmlns="http://www.w3.org/2000/svg"
      {...props}
    >
      <title>{title}</title>
      {bars.map((d) => (
        <path key={d} clipRule="evenodd" d={d} fill="currentColor" fillRule="evenodd" />
      ))}
      {dots.map(([x, y]) => (
        <rect key={`${x}:${y}`} fill="currentColor" height="1" rx=".5" width="1" x={x} y={y} />
      ))}
    </svg>
  )
}

export const BorderAllIcon = (props: TableIconProps) => (
  <BorderIcon title="Border All" edges={ALL_EDGES} {...props} />
)
export const BorderBottomIcon = (props: TableIconProps) => (
  <BorderIcon title="Border Bottom" edges={['bottom']} {...props} />
)
export const BorderLeftIcon = (props: TableIconProps) => (
  <BorderIcon title="Border Left" edges={['left']} {...props} />
)
export const BorderNoneIcon = (props: TableIconProps) => (
  <BorderIcon title="Border None" edges={[]} {...props} />
)
export const BorderRightIcon = (props: TableIconProps) => (
  <BorderIcon title="Border Right" edges={['right']} {...props} />
)
export const BorderTopIcon = (props: TableIconProps) => (
  <BorderIcon title="Border Top" edges={['top']} {...props} />
)
