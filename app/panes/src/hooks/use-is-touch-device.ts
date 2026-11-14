// Copied verbatim from the Plate registry (block-menu-kit -> use-is-touch-device). BlockContextMenu
// uses it to skip the right-click menu on touch devices.
'use client'

import * as React from 'react'

export function useIsTouchDevice() {
  const [isTouchDevice, setIsTouchDevice] = React.useState(false)

  React.useEffect(() => {
    function onResize() {
      setIsTouchDevice('ontouchstart' in window || navigator.maxTouchPoints > 0)
    }

    window.addEventListener('resize', onResize)
    onResize()

    return () => {
      window.removeEventListener('resize', onResize)
    }
  }, [])

  return isTouchDevice
}
