import { defaultSettings } from "@/features/settings/config/default-settings";
import { applySettingsSideEffects } from "@/features/settings/lib/settings-effects";
import { normalizeSettings } from "@/features/settings/lib/settings-normalization";
import {
  loadSettingsFromStore,
  saveSettingsToStore,
} from "@/features/settings/lib/settings-persistence";
import type { Settings } from "@/features/settings/types/settings";

function getSystemThemePreference(): "light" | "dark" {
  if (typeof window !== "undefined" && window.matchMedia) {
    try {
      return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
    } catch (error) {
      console.warn("matchMedia not available:", error);
    }
  }

  return "dark";
}


export async function resolveInitialSettings(): Promise<Settings> {
  if (typeof window === "undefined") {
    return defaultSettings;
  }

  const loadedSettings = await loadSettingsFromStore();

  // When system-sync is on, derive the active theme from the OS preference
  // using the user's chosen light/dark theme pair. This also covers the very
  // first launch (before any theme has been stored) so the app matches the
  // system out of the box.
  if (loadedSettings.syncSystemTheme || !loadedSettings.theme) {
    const systemPref = getSystemThemePreference();
    loadedSettings.theme =
      systemPref === "dark" ? loadedSettings.autoThemeDark : loadedSettings.autoThemeLight;
  }

  return normalizeSettings(loadedSettings);
}

export async function initializeSettingsState(
  applySettings: (settings: Settings) => void,
): Promise<Settings> {
  try {
    const normalizedSettings = await resolveInitialSettings();
    applySettingsSideEffects(normalizedSettings);
    applySettings(normalizedSettings);
    await saveSettingsToStore(normalizedSettings);
    return normalizedSettings;
  } catch (error) {
    console.error("Failed to initialize settings:", error);
    return defaultSettings;
  }
}
