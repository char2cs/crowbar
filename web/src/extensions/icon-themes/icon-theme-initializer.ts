import { iconThemeRegistry } from "./icon-theme-registry"
import type { IconThemeSource } from "./types"
import { classicIconTheme } from "./builtin/classic-theme"
import { materialIconTheme } from "./builtin/material-theme"
import { colorfulMaterialIconTheme } from "./builtin/colorful-material-theme"
import { compactIconTheme } from "./builtin/compact-theme"
import { minimalIconTheme } from "./builtin/minimal-theme"
import { noneIconTheme } from "./builtin/none-theme"

const BUILTIN_SOURCE: IconThemeSource = { extensionId: "builtin", isBundled: true }

export function initializeIconThemes() {
  iconThemeRegistry.registerTheme(classicIconTheme, BUILTIN_SOURCE)
  iconThemeRegistry.registerTheme(materialIconTheme, BUILTIN_SOURCE)
  iconThemeRegistry.registerTheme(colorfulMaterialIconTheme, BUILTIN_SOURCE)
  iconThemeRegistry.registerTheme(compactIconTheme, BUILTIN_SOURCE)
  iconThemeRegistry.registerTheme(minimalIconTheme, BUILTIN_SOURCE)
  iconThemeRegistry.registerTheme(noneIconTheme, BUILTIN_SOURCE)
}
