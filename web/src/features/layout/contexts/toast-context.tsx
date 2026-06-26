import type React from 'react'
import { toast } from '@/features/window/stores/toast-store'

interface ShowOptions {
  message: string
  description?: string
  type?: 'info' | 'success' | 'warning' | 'error'
  key?: string
  duration?: number
  action?: { label: string; onClick: () => void }
}

interface ToastContextType {
  showToast: (value: ShowOptions) => string
  dismissToast: (id: string) => void
}

export const ToastProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => children

export const useToast = (): ToastContextType => ({
  showToast: (value) => toast.show(value),
  dismissToast: (id) => toast.dismiss(id),
})
