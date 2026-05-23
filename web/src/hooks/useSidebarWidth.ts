import { useCallback, useEffect, useState } from 'react'

const MIN = 180
const MAX = 400
const DEFAULT = 256
const STORAGE_KEY = 'crowbar-sidebar-width'

function clamp(v: number) { return Math.min(MAX, Math.max(MIN, v)) }

export function useSidebarWidth() {
  const [width, setWidthState] = useState(() => {
    const stored = localStorage.getItem(STORAGE_KEY)
    return stored ? clamp(Number(stored)) : DEFAULT
  })

  useEffect(() => {
    localStorage.setItem(STORAGE_KEY, String(width))
  }, [width])

  const setWidth = useCallback((v: number) => setWidthState(clamp(v)), [])

  const startResize = useCallback(
    (e: React.MouseEvent) => {
      const startX = e.clientX
      const startW = width
      const onMove = (e: MouseEvent) => setWidthState(clamp(startW + e.clientX - startX))
      const onUp = () => {
        document.removeEventListener('mousemove', onMove)
        document.removeEventListener('mouseup', onUp)
        document.body.style.cursor = ''
        document.body.style.userSelect = ''
      }
      document.body.style.cursor = 'col-resize'
      document.body.style.userSelect = 'none'
      document.addEventListener('mousemove', onMove)
      document.addEventListener('mouseup', onUp)
      e.preventDefault()
    },
    [width],
  )

  return { width, setWidth, startResize }
}
