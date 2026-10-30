import type { NativePalette } from '@/lib/native-chrome'

const token = (style: CSSStyleDeclaration, name: string) => style.getPropertyValue(name).trim()

/** The sidebar's palette: what a `chrome-surface` region resolves to, or null in a theme whose
 *  sidebar keeps the plain chrome (told by having no raised controls). */
function readSidebarPalette(): NativePalette['sidebar'] {
  const probe = document.createElement('div')
  probe.className = 'chrome-surface'
  document.body.append(probe)
  const style = getComputedStyle(probe)
  const sidebar = token(style, '--control-surface')
    ? {
        text: token(style, '--foreground'),
        brand: token(style, '--primary'),
        brandText: token(style, '--primary-foreground'),
      }
    : null
  probe.remove()
  return sidebar
}

/** The current theme's palette as CSS gives it: the Settings page's surface and its cards. */
export function readNativePalette(): NativePalette {
  const style = getComputedStyle(document.documentElement)
  return {
    surface: token(style, '--background'),
    raised: token(style, '--muted'),
    text: token(style, '--foreground'),
    secondaryText: token(style, '--muted-foreground'),
    brandLight: token(style, '--brand-light'),
    brandDark: token(style, '--brand-dark'),
    sidebar: readSidebarPalette(),
  }
}
