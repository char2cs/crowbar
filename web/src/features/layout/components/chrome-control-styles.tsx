// Stub - window chrome control styles for macOS/Windows
import { cva } from 'class-variance-authority'

export function ChromeControlStyles() {
  return null
}
export function useChromeControlPadding(): number {
  return 0
}

export const chromeControl = cva('inline-flex items-center', {
  variants: {
    shape: {
      pill: 'rounded-full px-2',
      sidebar: 'rounded-md',
      tab: 'rounded-md',
    },
  },
})
export const chromeControlGroup = cva('inline-flex items-center gap-1')
export const chromeIcon = cva('size-4')
export const chromeItemWrapper = cva('inline-flex items-center')
