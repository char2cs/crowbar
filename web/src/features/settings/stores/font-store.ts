// Tauri font APIs replaced with stubs — font enumeration not available in web mode
import { create } from 'zustand'
import { immer } from 'zustand/middleware/immer'
import type { FontInfo } from '@/features/settings/stores/types/font'
import { createSelectors } from '@/utils/zustand-selectors'

interface FontState {
  availableFonts: FontInfo[]
  monospaceFonts: FontInfo[]
  actions: FontActions
}

interface FontActions {
  loadAvailableFonts: (forceRefresh?: boolean) => Promise<void>
  loadMonospaceFonts: (forceRefresh?: boolean) => Promise<void>
  validateFont: (fontFamily: string) => Promise<boolean>
}

const FONT_CACHE_KEY = 'crowbar_font_cache:v1'
const FONT_CACHE_EXPIRY = 24 * 60 * 60 * 1000 // 24 hours in milliseconds
// Fonts that ship with the app and are therefore always selectable, regardless
// of OS-level font enumeration (which is unavailable in the WKWebView).
// "JetBrains Mono Variable" is the default for both the editor and the terminal
// (see typography-defaults.ts); the static cut stays selectable alongside it.
const BUNDLED_FONTS: FontInfo[] = [
  {
    name: 'CalSansUI',
    family: 'CalSansUI',
    style: 'Regular',
    is_monospace: false,
  },
  {
    name: 'JetBrains Mono',
    family: 'JetBrains Mono',
    style: 'Regular',
    is_monospace: true,
  },
  {
    name: 'JetBrains Mono Variable',
    family: 'JetBrains Mono Variable',
    style: 'Regular',
    is_monospace: true,
  },
]

interface FontCache {
  availableFonts: FontInfo[]
  monospaceFonts: FontInfo[]
  timestamp: number
}

const loadFontsFromCache = (): FontCache | null => {
  try {
    const cached = localStorage.getItem(FONT_CACHE_KEY)
    if (!cached) return null

    const cache: FontCache = JSON.parse(cached)
    const now = Date.now()

    // Check if cache is expired
    if (now - cache.timestamp > FONT_CACHE_EXPIRY) {
      localStorage.removeItem(FONT_CACHE_KEY)
      return null
    }

    return cache
  } catch (error) {
    console.error('Failed to load fonts from cache:', error)
    localStorage.removeItem(FONT_CACHE_KEY)
    return null
  }
}

const saveFontsToCache = (availableFonts: FontInfo[], monospaceFonts: FontInfo[]) => {
  try {
    const cache: FontCache = {
      availableFonts,
      monospaceFonts,
      timestamp: Date.now(),
    }
    localStorage.setItem(FONT_CACHE_KEY, JSON.stringify(cache))
  } catch (error) {
    console.error('Failed to save fonts to cache:', error)
  }
}

export const useFontStore = createSelectors(
  create<FontState>()(
    immer((set, get) => {
      // Try to load from cache immediately
      const cache = loadFontsFromCache()
      const initialValues = cache
        ? {
            availableFonts: cache.availableFonts,
            monospaceFonts: cache.monospaceFonts,
          }
        : {
            availableFonts: [],
            monospaceFonts: [],
          }

      return {
        ...initialValues,
        actions: {
          loadAvailableFonts: async (forceRefresh = false) => {
            const current = get()

            // Use cached data if available and not forcing refresh
            // But only if we have more than just the web fonts
            if (!forceRefresh && current.availableFonts.length > 1) {
              return
            }

            // OS font enumeration is unavailable in the WKWebView, so the
            // selectable set is the fonts the app bundles.
            const fonts: FontInfo[] = BUNDLED_FONTS
            const monospaceFonts = fonts.filter((font) => font.is_monospace)

            set((state) => {
              state.availableFonts = fonts
              state.monospaceFonts = monospaceFonts
            })

            saveFontsToCache(fonts, monospaceFonts)
          },

          loadMonospaceFonts: async (forceRefresh = false) => {
            const current = get()

            // Use cached data if available and not forcing refresh
            // Ensure we have actual fonts loaded
            if (!forceRefresh && current.monospaceFonts.length > 0) {
              return
            }

            const fonts: FontInfo[] = BUNDLED_FONTS.filter((font) => font.is_monospace)

            set((state) => {
              state.monospaceFonts = fonts
            })

            // Cache needs the full list, so only write once it has been loaded.
            const updatedState = get()
            if (updatedState.availableFonts.length > 0) {
              saveFontsToCache(updatedState.availableFonts, fonts)
            }
          },

          // Every selectable font is bundled with the app (see BUNDLED_FONTS),
          // so anything the picker can offer is by definition installed.
          validateFont: async (): Promise<boolean> => true,
        },
      }
    }),
  ),
)
