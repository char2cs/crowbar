import { createContext, use, type CSSProperties } from 'react'
import { ROW_INDENT_STEP } from '@/components/layout/workspace-row-base'

/** A row scale that replaces the desktop's dense one wholesale (sizes in px). */
export interface RowMetrics {
  height: number
  titleSize: number
  /** The second line under a title: a branch name and its counts. */
  subSize: number
  glyph: number
  /** A repo header's icon tile. */
  headerGlyph: number
  /** The least a control's tap target may be, however small its mark. */
  hitTarget: number
  indent: number
  gap: number
  padX: number
}

/** The phone's list scale: 34pt rows, 15px titles, one 12px secondary size. */
export const TOUCH_ROW_METRICS: RowMetrics = {
  height: 34,
  titleSize: 15,
  subSize: 12,
  glyph: 16,
  headerGlyph: 24,
  hitTarget: 44,
  indent: 14,
  gap: 8,
  padX: 8,
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

export function rowHeaderGlyphStyle(metrics: RowMetrics | null): CSSProperties | undefined {
  return metrics ? { width: metrics.headerGlyph, height: metrics.headerGlyph } : undefined
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
