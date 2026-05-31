import type { ReactNode } from 'react'
import type { WidgetData } from '../types'

export interface WidgetComponentProps {
  data: WidgetData
  onChange: (payload: unknown) => void
}

export type WidgetComponent = (props: WidgetComponentProps) => ReactNode

// Populated by excalidraw-widget.tsx and mermaid-widget.tsx via registerWidget()
export const WIDGET_REGISTRY: Record<string, WidgetComponent> = {}

export function registerWidget(type: string, component: WidgetComponent): void {
  WIDGET_REGISTRY[type] = component
}

export function getWidget(type: string): WidgetComponent | undefined {
  return WIDGET_REGISTRY[type]
}
