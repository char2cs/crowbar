import { useEffect, useState } from 'react'
import {
  Warning as AlertTriangle,
  CheckCircle as CheckCircle2,
  Info,
  X,
} from '@phosphor-icons/react'
import { Toaster as SonnerToaster } from 'sonner'
import { LoadingSpinner } from '@/components/ui/loading-spinner'
import { toast, useToastStore } from '@/features/window/stores/toast-store'

export type { Toast, NotificationEntry } from '@/features/window/stores/toast-store'
export { toast, useToastStore } from '@/features/window/stores/toast-store'

export const useToast = () => {
  const toasts = useToastStore.use.toasts()
  const notifications = useToastStore.use.notifications()

  return {
    toasts,
    notifications,
    showToast: toast.show,
    updateToast: toast.update,
    dismissToast: toast.dismiss,
    dismissToastByKey: toast.dismissByKey,
    hasToast: toast.has,
    toast,
  }
}

function getToastTheme() {
  if (typeof document === 'undefined') return 'dark'
  return document.documentElement.getAttribute('data-theme-type') === 'light' ? 'light' : 'dark'
}

export const ToastContainer = () => {
  const [theme, setTheme] = useState<'light' | 'dark'>(getToastTheme)

  useEffect(() => {
    const root = document.documentElement
    const observer = new MutationObserver(() => {
      setTheme(getToastTheme())
    })

    observer.observe(root, {
      attributes: true,
      attributeFilter: ['data-theme-type'],
    })

    setTheme(getToastTheme())

    return () => {
      observer.disconnect()
    }
  }, [])

  return (
    <SonnerToaster
      position="bottom-right"
      expand
      theme={theme}
      icons={{
        success: <CheckCircle2 size={18} />,
        info: <Info size={18} />,
        warning: <AlertTriangle size={18} />,
        error: <AlertTriangle size={18} />,
        loading: <LoadingSpinner label="Loading" compact />,
        close: <X size={14} />,
      }}
      toastOptions={{
        closeButton: true,
        className: 'ui-font font-normal group',
        descriptionClassName: 'ui-font font-normal',
        classNames: {
          toast:
            'group ui-font rounded-xl border border-border bg-primary-bg text-text font-normal shadow-xl backdrop-blur-sm',
          content: 'pr-8',
          title: 'ui-font ui-text-sm font-normal leading-5 text-text',
          description: 'ui-font ui-text-sm font-normal leading-5 text-text-light',
          icon: 'mt-0.5',
          success: 'border-border',
          info: 'border-border',
          warning: 'border-border',
          error: 'border-border',
          loading: 'border-border',
          closeButton:
            'absolute left-auto right-2 top-2 m-0 opacity-0 transition-opacity group-hover:opacity-100 border-none bg-transparent text-text-lighter hover:bg-hover hover:text-text',
          actionButton: 'ui-font border-none bg-hover text-text hover:bg-border',
          cancelButton: 'ui-font border-none bg-hover text-text hover:bg-border',
        },
        actionButtonStyle: {
          background: 'var(--color-hover)',
          color: 'var(--color-text)',
        },
        cancelButtonStyle: {
          background: 'var(--color-hover)',
          color: 'var(--color-text)',
        },
        style: {
          background: 'var(--color-primary-bg)',
          border: '1px solid var(--color-border)',
          color: 'var(--color-text)',
          fontFamily: 'var(--font-ui)',
          fontWeight: '400',
        },
      }}
    />
  )
}
