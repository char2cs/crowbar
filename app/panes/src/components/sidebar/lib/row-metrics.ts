import { createContext, use, type CSSProperties } from 'react'
import { ROW_INDENT_STEP } from '@/components/layout/workspace-row-base'

export interface RowMetrics {
  height: number
  titleSize: number
  subSize: number
  glyph: number
  hitTarget: number
  indent: number
  gap: number
  padX: number
  projectHeight: number
  projectTile: number
  projectTitleSize: number
}

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

export function rowTitleStyle(metrics: RowMetrics | null): CSSProperties | undefined {
  return metrics ? { lineHeight: `${metrics.titleSize + 4}px` } : undefined
}
