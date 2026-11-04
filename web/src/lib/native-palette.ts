import { cssColorToHex } from '@/features/editor/theme/resolve-css-color'
import type { NativePalette } from '@/lib/native-chrome'

/** A token as a colour the phone parses: the browser resolves any CSS colour function or
 *  `var()` chain on a child of `host`, then it is sent as hex. */
function colorOf(host: Element, name: string): string {
  const raw = getComputedStyle(host).getPropertyValue(name).trim()
  const probe = document.createElement('span')
  probe.style.color = raw
  host.append(probe)
  const resolved = getComputedStyle(probe).color
  probe.remove()
  return cssColorToHex(resolved || raw) ?? (resolved || raw)
}

const isLight = (hex: string) => {
  const [r, g, b] = [1, 3, 5].map((i) => Number.parseInt(hex.slice(i, i + 2), 16))
  return (r * 299 + g * 587 + b * 114) / 1000 > 128
}

/** The sidebar's palette: what a `chrome-surface` region resolves to, or null where it keeps
 *  the plain chrome (told by its text being dark, not light, ink). */
function readSidebarPalette(): NativePalette['sidebar'] {
  const probe = document.createElement('div')
  probe.className = 'chrome-surface'
  document.body.append(probe)
  const text = colorOf(probe, '--foreground')
  const sidebar =
    text.startsWith('#') && isLight(text)
      ? {
          text,
          brand: colorOf(probe, '--primary'),
          brandText: colorOf(probe, '--primary-foreground'),
        }
      : null
  probe.remove()
  return sidebar
}

/** The current theme's palette as CSS gives it: the Settings page's surface and its cards. */
export function readNativePalette(): NativePalette {
  const root = document.documentElement
  return {
    surface: colorOf(root, '--background'),
    raised: colorOf(root, '--muted'),
    text: colorOf(root, '--foreground'),
    secondaryText: colorOf(root, '--muted-foreground'),
    brandLight: colorOf(root, '--brand-light'),
    brandDark: colorOf(root, '--brand-dark'),
    sidebar: readSidebarPalette(),
  }
}
