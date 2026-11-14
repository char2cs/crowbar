import { createContext, use, type CSSProperties } from 'react'
import { ROW_INDENT_STEP } from '@/components/layout/workspace-row-base'

/** A row scale that replaces the desktop's dense one wholesale (sizes in px). */
export interface RowMetrics {
  height: number
  titleSize: number
  /** The second line under a title: a branch name and its counts. */
  subSize: number
  /** Every row's mark: a repo's logo, a branch's, a thread's and a folder's alike. */
  glyph: number
  /** The least a control's tap target may be, however small its mark. */
  hitTarget: number
  indent: number
  gap: number
  padX: number
  /** The project header above the list: its height, mark tile and name size. */
  projectHeight: number
  projectTile: number
  projectTitleSize: number
}

/** The phone's list scale: 40pt rows, 16px titles, one 12px secondary size, 20px marks. */
export const TOUCH_ROW_METRICS: RowMetrics = {
  height: 40,
  titleSize: 16,
  subSize: 12,
  glyph: 20,
  hitTarget: 44,
  indent: 16,
  gap: 10,
  padX: 10,
  projectHeight: 56,
  projectTile: 36,
  projectTitleSize: 22,
}

/** Null keeps the desktop's own scale, which lives in the row-base classes. */
export const RowMetricsContext = createContext<RowMetrics | null>(null)

export function useRowMetrics(): RowMetrics | null {
  return use(RowMetricsContext)
}

export function rowIndent(depth: number, metrics: RowMetrics | null): number {
  return depth * (metrics?.indent ?? ROW_INDENT_STEP)
}

export function rowBoxStyle(metrics: RowMetrics | null): CSSProperties | undefined {
  if (!metrics) return undefined
  return {
    height: metrics.height,
    fontSize: metrics.titleSize,
    gap: metrics.gap,
    paddingInline: metrics.padX,
  }
}

export function rowGlyphStyle(metrics: RowMetrics | null): CSSProperties | undefined {
  return metrics ? { width: metrics.glyph, height: metrics.glyph } : undefined
}

/** A tap target taller than the row, bled into its neighbours so the row keeps its height. */
export function rowHitStyle(metrics: RowMetrics | null): CSSProperties | undefined {
  if (!metrics) return undefined
  const bleed = (metrics.height - metrics.hitTarget) / 2
  return {
    width: metrics.hitTarget,
    height: metrics.hitTarget,
    marginBlock: bleed,
    marginInlineEnd: -metrics.padX,
  }
}

export function rowSubStyle(metrics: RowMetrics | null): CSSProperties | undefined {
  return metrics ? { fontSize: metrics.subSize, lineHeight: `${metrics.subSize + 3}px` } : undefined
}

/** A row's title line when a second line sits under it: both must fit the row's own height. */
export function rowTitleStyle(metrics: RowMetrics | null): CSSProperties | undefined {
  return metrics ? { lineHeight: `${metrics.titleSize + 4}px` } : undefined
}
