import { iconThemeRegistry } from "./icon-theme-registry"
import { classicIconTheme } from "./builtin/classic-theme"
import { materialIconTheme } from "./builtin/material-theme"
import { colorfulMaterialIconTheme } from "./builtin/colorful-material-theme"
import { compactIconTheme } from "./builtin/compact-theme"
import { minimalIconTheme } from "./builtin/minimal-theme"
import { noneIconTheme } from "./builtin/none-theme"

export function initializeIconThemes() {
  iconThemeRegistry.registerTheme(classicIconTheme)
  iconThemeRegistry.registerTheme(materialIconTheme)
  iconThemeRegistry.registerTheme(colorfulMaterialIconTheme)
  iconThemeRegistry.registerTheme(compactIconTheme)
  iconThemeRegistry.registerTheme(minimalIconTheme)
  iconThemeRegistry.registerTheme(noneIconTheme)
}
