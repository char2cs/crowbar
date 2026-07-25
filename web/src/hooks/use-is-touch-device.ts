// Generated from the Plate registry (`https://platejs.org/r/block-menu-kit.json`
// -> `use-is-touch-device`), copied verbatim — no primitive collision, no
// trimming needed. Used by BlockContextMenu to skip the right-click menu on
// touch devices (long-press semantics differ; out of scope here).
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
