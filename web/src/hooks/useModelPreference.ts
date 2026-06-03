import { useState } from 'react'

export const MODELS = [
  { id: 'claude-haiku-4-5-20251001', label: 'Haiku 4.5' },
  { id: 'claude-sonnet-4-6', label: 'Sonnet 4.6' },
  { id: 'claude-opus-4-7', label: 'Opus 4.7' },
] as const

export type ModelId = typeof MODELS[number]['id']

const STORAGE_KEY = 'crowbar.model'
const DEFAULT_MODEL: ModelId = 'claude-sonnet-4-6'

export function useModelPreference() {
  const [model, setModelState] = useState<ModelId>(() => {
    try {
      const stored = localStorage.getItem(STORAGE_KEY)
      return (MODELS.find(m => m.id === stored)?.id ?? DEFAULT_MODEL)
    } catch {
      return DEFAULT_MODEL
    }
  })

  const setModel = (id: ModelId) => {
    try {
      localStorage.setItem(STORAGE_KEY, id)
    } catch {
      // storage unavailable — preference not persisted this session
    }
    setModelState(id)
  }

  const modelLabel = MODELS.find(m => m.id === model)?.label ?? 'Sonnet 4.6'

  return { model, setModel, modelLabel, models: MODELS }
}
