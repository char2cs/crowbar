import { useEffect, useEffectEvent, useState } from 'react'
import type { AgentProvider } from '@/features/agent/api/agent-api'
import {
  onNativeChromeIntent,
  setNativeModelPicker,
  type NativeChromeIntent,
} from '@/lib/native-chrome'
import { effortLevelsFor, resolveEffort } from './agent-selection-efforts'

const ICON_PX = 60

// The native sheet draws bitmaps: each provider's SVG icon, rasterised once.
function rasterize(svg: string): Promise<string | null> {
  return new Promise((resolve) => {
    const image = new Image()
    image.onload = () => {
      const canvas = document.createElement('canvas')
      canvas.width = ICON_PX
      canvas.height = ICON_PX
      const context = canvas.getContext('2d')
      if (!context) return resolve(null)
      context.drawImage(image, 0, 0, ICON_PX, ICON_PX)
      resolve(canvas.toDataURL('image/png'))
    }
    image.onerror = () => resolve(null)
    image.src = `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`
  })
}

function useProviderIcons(providers: AgentProvider[]): Record<string, string | null> {
  const [icons, setIcons] = useState<Record<string, string | null>>({})
  // Keyed by the icons themselves, not the array's identity.
  const key = JSON.stringify(providers.map((p) => [p.id, p.icon ?? '']))
  useEffect(() => {
    let live = true
    const pairs = JSON.parse(key) as [string, string][]
    void Promise.all(
      pairs.map(async ([id, svg]) => [id, svg ? await rasterize(svg) : null] as const),
    ).then((entries) => {
      if (live) setIcons(Object.fromEntries(entries))
    })
    return () => {
      live = false
    }
  }, [key])
  return icons
}

interface NativeModelPickerProps {
  open: boolean
  /** The offered providers with a model catalogue, in order. */
  providers: AgentProvider[]
  provider: AgentProvider | undefined
  model: string
  effort: string
  onSelectionChange: (provider: string, model: string, effort: string) => void
  onClose: () => void
}

/** The model and effort picker as the phone's native sheet: the catalogue pushed whole while it
 *  is open, picks staged exactly as the desktop menu stages them. */
export function NativeModelPicker({
  open,
  providers,
  provider,
  model,
  effort,
  onSelectionChange,
  onClose,
}: NativeModelPickerProps) {
  const icons = useProviderIcons(providers)

  useEffect(() => {
    if (!open) return
    setNativeModelPicker({
      open,
      providers: providers.map((p) => ({
        id: p.id,
        name: p.displayName,
        icon: icons[p.id] ?? null,
        symbol: 'sparkles',
        models: (p.models ?? []).map((m) => ({ id: m, name: m, detail: null })),
      })),
      selected: provider && model ? { providerId: provider.id, modelId: model } : null,
      efforts: effortLevelsFor(provider, model),
      effort: effort || null,
    })
  }, [open, providers, icons, provider, model, effort])

  // Only the open picker speaks: a closed one says so once, and never again.
  useEffect(() => {
    if (!open) return
    return () => setNativeModelPicker(CLOSED)
  }, [open])

  const onIntent = useEffectEvent((intent: NativeChromeIntent) => {
    if (intent.type === 'selectModel') {
      const target = providers.find((p) => p.id === intent.providerId)
      if (!target) return
      onSelectionChange(target.id, intent.modelId, resolveEffort(target, intent.modelId, effort))
      onClose()
    }
    if (intent.type === 'setEffort') onSelectionChange(provider?.id ?? '', model, intent.effort)
    if (intent.type === 'closeModelPicker') onClose()
  })
  useEffect(() => {
    if (!open) return
    return onNativeChromeIntent(onIntent)
  }, [open])
  return null
}

const CLOSED = { open: false, providers: [], selected: null, efforts: [], effort: null }
