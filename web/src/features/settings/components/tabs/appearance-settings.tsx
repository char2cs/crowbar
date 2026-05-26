import { Upload } from "@phosphor-icons/react";
import { useEffect, useMemo, useState } from "react";
import { iconThemeRegistry } from "@/extensions/icon-themes/icon-theme-registry";
import { themeRegistry } from "@/extensions/themes/theme-registry";
import {
  formatUiFontSize,
  UI_FONT_SIZE_MAX,
  UI_FONT_SIZE_MIN,
  UI_FONT_SIZE_STEP,
} from "@/features/settings/lib/ui-font-size";
import { getDefaultSetting, useSettingsStore } from "@/features/settings/store";
import { Button } from "@/components/ui/button";
import NumberInput from "@/components/ui/number-input";
import Section, { SETTINGS_CONTROL_WIDTHS, SettingRow } from "../settings-section";
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "@/components/ui/select";
import { cn } from "@/utils/cn";
import { FontSelector } from "../font-selector";
import type { Theme, ThemeMode } from "@/features/settings/types/settings";

const THEME_MODE_OPTIONS: { value: ThemeMode; label: string }[] = [
  { value: "system", label: "Sync with System" },
  { value: "light",  label: "Light" },
  { value: "dark",   label: "Dark" },
];

const SIDEBAR_POSITION_OPTIONS: { value: "left" | "right"; label: string }[] = [
  { value: "left", label: "Left" },
  { value: "right", label: "Right" },
]

export const AppearanceSettings = () => {
  const { settings, updateSetting } = useSettingsStore();
  const [themeOptions, setThemeOptions] = useState<{ value: string; label: string }[]>([]);
  const [iconThemeOptions, setIconThemeOptions] = useState<{ value: string; label: string }[]>([]);

  // Load color themes from registry
  useEffect(() => {
    const load = () => {
      const options = themeRegistry.getAllThemes().map((theme) => ({
        value: theme.id,
        label: theme.name,
      }));
      setThemeOptions(options);
    };
    load();
    return themeRegistry.onRegistryChange(load);
  }, []);

  // Ensure the current theme appears even if it's not in the list yet
  const normalizedThemeOptions = useMemo(() => {
    if (themeOptions.some((o) => o.value === settings.theme)) return themeOptions;
    const fallback = themeRegistry.getTheme(settings.theme);
    if (!fallback) return themeOptions;
    return [{ value: fallback.id, label: fallback.name }, ...themeOptions];
  }, [themeOptions, settings.theme]);

  // Load icon themes from registry
  useEffect(() => {
    const load = () => {
      const options = iconThemeRegistry.getAllThemes().map((theme) => ({
        value: theme.id,
        label: theme.name,
      }));
      setIconThemeOptions(options);
    };
    load();
    return iconThemeRegistry.onRegistryChange(load);
  }, []);

  const normalizedIconThemeOptions = useMemo(() => {
    if (iconThemeOptions.some((o) => o.value === settings.iconTheme)) return iconThemeOptions;
    const fallback = iconThemeRegistry.getTheme(settings.iconTheme);
    if (!fallback) return iconThemeOptions;
    return [{ value: fallback.id, label: fallback.name }, ...iconThemeOptions];
  }, [iconThemeOptions, settings.iconTheme]);

  const handleUploadTheme = () => {
    const input = document.createElement("input")
    input.type = "file"
    input.accept = ".json"
    input.style.display = "none"
    document.body.appendChild(input)
    input.onchange = async (e) => {
      const file = (e.target as HTMLInputElement).files?.[0]
      input.remove()
      if (file) {
        const { uploadTheme } = await import("@/features/settings/utils/theme-upload")
        const result = await uploadTheme(file)
        if (!result.success) {
          console.error("Theme upload failed:", result.error)
        }
      }
    }
    input.click()
  }

  return (
    <div className="space-y-4">
      <Section title="Theme">
        <SettingRow
          label="Color Theme"
          description="Choose your active color theme"
          onReset={() => updateSetting("theme", getDefaultSetting("theme"))}
          canReset={settings.theme !== getDefaultSetting("theme")}
        >
          <div className="flex items-center gap-2">
            <Select value={settings.theme} onValueChange={(value) => { if (value) updateSetting("theme", value as Theme) }}>
              <SelectTrigger size="sm" className={SETTINGS_CONTROL_WIDTHS.wide}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {normalizedThemeOptions.map((opt) => (
                  <SelectItem key={opt.value} value={opt.value}>
                    {opt.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Button
              type="button"
              onClick={handleUploadTheme}
              variant="default"
              tooltip="Upload theme"
              aria-label="Upload theme"
              compact
            >
              <Upload />
            </Button>
          </div>
        </SettingRow>

        <SettingRow
          label="Theme Mode"
          description="Use light, dark, or follow system preference"
          onReset={() => updateSetting("themeMode", getDefaultSetting("themeMode"))}
          canReset={settings.themeMode !== getDefaultSetting("themeMode")}
        >
          <Select value={settings.themeMode} onValueChange={(value) => { if (value) updateSetting("themeMode", value as ThemeMode) }}>
            <SelectTrigger size="sm" className={SETTINGS_CONTROL_WIDTHS.wide}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {THEME_MODE_OPTIONS.map((opt) => (
                <SelectItem key={opt.value} value={opt.value}>
                  {opt.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </SettingRow>
      </Section>

      <Section title="Icons">
        <SettingRow
          label="Icon Theme"
          description="Icons displayed in the file tree and tabs"
          onReset={() => updateSetting("iconTheme", getDefaultSetting("iconTheme"))}
          canReset={settings.iconTheme !== getDefaultSetting("iconTheme")}
        >
          <Select value={settings.iconTheme} onValueChange={(value) => { if (value) updateSetting("iconTheme", value) }}>
            <SelectTrigger size="sm" className={SETTINGS_CONTROL_WIDTHS.wide}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {normalizedIconThemeOptions.map((opt) => (
                <SelectItem key={opt.value} value={opt.value}>
                  {opt.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </SettingRow>
      </Section>

      <Section title="Typography">
        <SettingRow
          label="UI Font Family"
          description="Font family for UI elements (file tree, markdown, etc.)"
          onReset={() => updateSetting("uiFontFamily", getDefaultSetting("uiFontFamily"))}
          canReset={settings.uiFontFamily !== getDefaultSetting("uiFontFamily")}
        >
          <FontSelector
            value={settings.uiFontFamily}
            onChange={(fontFamily) => updateSetting("uiFontFamily", fontFamily)}
            className={SETTINGS_CONTROL_WIDTHS.text}
            monospaceOnly={false}
          />
        </SettingRow>

        <SettingRow
          label="UI Font Size"
          description="Adjust UI text and icon scale in 0.5px steps"
          onReset={() => updateSetting("uiFontSize", getDefaultSetting("uiFontSize"))}
          canReset={settings.uiFontSize !== getDefaultSetting("uiFontSize")}
        >
          <NumberInput
            min={String(UI_FONT_SIZE_MIN)}
            max={String(UI_FONT_SIZE_MAX)}
            step={String(UI_FONT_SIZE_STEP)}
            value={settings.uiFontSize}
            onChange={(value) => updateSetting("uiFontSize", value)}
            className={cn(SETTINGS_CONTROL_WIDTHS.number, "tabular-nums")}
            size="xs"
            aria-label={`UI font size: ${formatUiFontSize(settings.uiFontSize)} pixels`}
          />
        </SettingRow>
      </Section>

      <Section title="Layout">
        <SettingRow
          label="Sidebar Position"
          description="Which side the file sidebar appears on"
          onReset={() => updateSetting("sidebarPosition", getDefaultSetting("sidebarPosition"))}
          canReset={settings.sidebarPosition !== getDefaultSetting("sidebarPosition")}
        >
          <Select value={settings.sidebarPosition} onValueChange={(value) => { if (value) updateSetting("sidebarPosition", value as "left" | "right") }}>
            <SelectTrigger size="sm" className={SETTINGS_CONTROL_WIDTHS.compact}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {SIDEBAR_POSITION_OPTIONS.map((opt) => (
                <SelectItem key={opt.value} value={opt.value}>
                  {opt.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </SettingRow>
      </Section>
    </div>
  );
};
