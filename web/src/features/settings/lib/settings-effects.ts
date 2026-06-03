import {
  cacheFontsForBootstrap,
  cacheThemeForBootstrap,
} from "@/features/settings/lib/appearance-bootstrap";
import { setWindowTransparency, setMacOSWindowAppearance } from '@/lib/crowbar-bridge';
import type { Settings, Theme } from "@/features/settings/types/settings";

const ALL_THEME_CLASSES = [
  "force-crowbar-light",
  "force-crowbar-dark",
  "force-vitesse-light",
  "force-vitesse-dark",
];

function applyFallbackTheme(theme: Theme) {
  ALL_THEME_CLASSES.forEach((cls) => document.documentElement.classList.remove(cls));
  document.documentElement.classList.add(`force-${theme}`);
  // Keep .dark in sync so Tailwind dark: variants still apply in error cases.
  document.documentElement.classList.toggle("dark", !theme.includes("light"));
}

type SystemThemePreference = "light" | "dark";

interface LegacyMediaQueryList extends MediaQueryList {
  addListener(listener: (event: MediaQueryListEvent) => void): void;
  removeListener(listener: (event: MediaQueryListEvent) => void): void;
}

let removeThemeSyncListener: (() => void) | null = null;

function applyWindowTransparency(enabled: boolean) {
  if (typeof document === "undefined") return;

  document.documentElement.setAttribute(
    "data-window-transparency",
    enabled ? "enabled" : "disabled",
  );

  void setWindowTransparency(enabled).catch((error) => {
    console.warn("Failed to sync window transparency", error);
  });
}

function getSystemThemePreference(): SystemThemePreference {
  if (typeof window !== "undefined" && window.matchMedia) {
    try {
      return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
    } catch (error) {
      console.warn("matchMedia not available:", error);
    }
  }

  return "dark";
}

function applyThemeMode(themeMode: "light" | "dark" | "system") {
  if (typeof document === "undefined") return;
  if (themeMode === "system") {
    const systemIsDark = getSystemThemePreference() === "dark";
    document.documentElement.classList.toggle("dark", systemIsDark);
  } else {
    document.documentElement.classList.toggle("dark", themeMode === "dark");
  }
}

function stopSystemThemeSync() {
  removeThemeSyncListener?.();
  removeThemeSyncListener = null;
}

function syncThemeWithSystem(themeMode: "light" | "dark" | "system", onModeChanged?: () => void) {
  if (typeof window === "undefined" || !window.matchMedia) return;
  if (themeMode !== "system") {
    stopSystemThemeSync();
    return;
  }

  const mediaQuery = window.matchMedia("(prefers-color-scheme: dark)");
  stopSystemThemeSync();

  const handleChange = () => {
    applyThemeMode("system");
    onModeChanged?.();
  };

  if ("addEventListener" in mediaQuery) {
    mediaQuery.addEventListener("change", handleChange);
    removeThemeSyncListener = () => mediaQuery.removeEventListener("change", handleChange);
  } else {
    const legacyMediaQuery = mediaQuery as LegacyMediaQueryList;
    legacyMediaQuery.addListener(handleChange);
    removeThemeSyncListener = () => legacyMediaQuery.removeListener(handleChange);
  }
}

export async function applyTheme(theme: Theme) {
  if (typeof window === "undefined") return;

  try {
    const { themeRegistry } = await import("@/extensions/themes/theme-registry");

    if (!themeRegistry.isRegistryReady()) {
      themeRegistry.onReady(() => {
        themeRegistry.applyTheme(theme);
        const appliedTheme = themeRegistry.getTheme(theme);
        if (appliedTheme) {
          cacheThemeForBootstrap(appliedTheme);
          syncMacOSWindowAppearance(appliedTheme.isDark ? "dark" : "light");
        } else {
          const isDark = !theme.includes("light");
          document.documentElement.classList.toggle("dark", isDark);
          syncMacOSWindowAppearance(isDark ? "dark" : "light");
        }
      });
      return;
    }

    themeRegistry.applyTheme(theme);
    const appliedTheme = themeRegistry.getTheme(theme);
    if (appliedTheme) {
      cacheThemeForBootstrap(appliedTheme);
      syncMacOSWindowAppearance(appliedTheme.isDark ? "dark" : "light");
    } else {
      // Registry is a stub or doesn't recognise this theme yet.
      // Derive dark/light from the theme name so Tailwind dark: variants apply correctly.
      const isDark = !theme.includes("light");
      document.documentElement.classList.toggle("dark", isDark);
      syncMacOSWindowAppearance(isDark ? "dark" : "light");
    }
  } catch (error) {
    console.error("Failed to apply theme via registry:", error);
    applyFallbackTheme(theme);
  }
}

function syncMacOSWindowAppearance(themeType: "light" | "dark") {
  const transparencyEnabled =
    typeof document === "undefined"
      ? true
      : document.documentElement.getAttribute("data-window-transparency") !== "disabled";

  void setMacOSWindowAppearance(themeType, transparencyEnabled).catch((error) => {
    console.warn("Failed to sync macOS window appearance", error);
  });
}

export function cacheFontSettings(
  settings: Pick<Settings, "fontFamily" | "uiFontFamily" | "uiFontSize">,
) {
  cacheFontsForBootstrap(settings.fontFamily, settings.uiFontFamily, settings.uiFontSize);
}

export function applySettingsSideEffects(settings: Settings) {
  cacheFontSettings(settings);
  applyWindowTransparency(settings.windowTransparency);
  applyThemeMode(settings.themeMode);
  void applyTheme(settings.theme);
  syncThemeWithSystem(settings.themeMode, () => void applyTheme(settings.theme));
}

export function applySettingSideEffect<K extends keyof Settings>(
  key: K,
  value: Settings[K],
  getSettings: () => Settings,
) {
  if (key === "theme") {
    const settings = getSettings();
    void applyTheme(settings.theme);
    applyThemeMode(settings.themeMode);
  }

  if (key === "themeMode") {
    const settings = getSettings();
    applyThemeMode(settings.themeMode);
    syncThemeWithSystem(settings.themeMode, () => void applyTheme(settings.theme));
    // Re-apply the theme's color palette for the new light/dark mode
    void applyTheme(settings.theme);
  }

  if (key === "fontFamily" || key === "uiFontFamily" || key === "uiFontSize") {
    cacheFontSettings(getSettings());
  }

  if (key === "windowTransparency") {
    applyWindowTransparency(value as boolean);
  }
}
