import { resolveCssVar } from '@/features/editor/theme/resolve-css-color'
import { isDarkMode, useThemeVersion } from '@/features/editor/theme/use-theme-version'

export { useThemeVersion as useMermaidThemeVersion }

// resolveCssVar can return null (before theme.css paints, or with no stylesheet), so each token
// has a light/dark literal rather than letting mermaid get `undefined`.
function color(name: string, light: string, dark: string): string {
  return resolveCssVar(name) ?? (isDarkMode() ? dark : light)
}

// `resolveCssVar` only resolves colors; `--font-sans` is a font stack, so it gets the same
// "assign to a real property, read the computed value" trick on `font-family`.
function resolveFontFamily(): string {
  if (typeof document === 'undefined') return 'inherit'
  const probe = document.createElement('span')
  probe.style.fontFamily = 'var(--font-sans)'
  document.body.appendChild(probe)
  const resolved = getComputedStyle(probe).fontFamily
  document.body.removeChild(probe)
  return resolved || 'inherit'
}

/** Mermaid `theme: 'base'` variables taken wholly from the app's CSS tokens, so a diagram
 *  matches the painted theme. Pure; rebuilt before every render, so a theme change shows on the
 *  next one. */
export function buildMermaidThemeVariables(): Record<string, string> {
  return {
    background: color('--background', '#faf9f5', '#141413'),
    fontFamily: resolveFontFamily(),
    primaryColor: color('--secondary', '#ece9df', '#2a2a28'),
    primaryTextColor: color('--foreground', '#141413', '#f5f5f5'),
    primaryBorderColor: color('--border', '#e5e3da', '#3a3a38'),
    secondaryColor: color('--muted', '#f5f4ee', '#242422'),
    secondaryTextColor: color('--foreground', '#141413', '#f5f5f5'),
    secondaryBorderColor: color('--border', '#e5e3da', '#3a3a38'),
    tertiaryColor: color('--accent', '#f5f4ee', '#242422'),
    tertiaryTextColor: color('--foreground', '#141413', '#f5f5f5'),
    tertiaryBorderColor: color('--border', '#e5e3da', '#3a3a38'),
    lineColor: color('--muted-foreground', '#7c7b74', '#a3a29c'),
    textColor: color('--foreground', '#141413', '#f5f5f5'),
    mainBkg: color('--card', '#ffffff', '#1c1c1a'),
    nodeTextColor: color('--foreground', '#141413', '#f5f5f5'),
    edgeLabelBackground: color('--background', '#faf9f5', '#141413'),
    clusterBkg: color('--muted', '#f5f4ee', '#242422'),
    clusterBorder: color('--border', '#e5e3da', '#3a3a38'),
    defaultLinkColor: color('--muted-foreground', '#7c7b74', '#a3a29c'),
    titleColor: color('--foreground', '#141413', '#f5f5f5'),
    errorBkgColor: color('--destructive', '#ef4444', '#ef4444'),
    errorTextColor: color('--destructive-foreground', '#ffffff', '#ffffff'),
    noteBkgColor: color('--accent', '#f5f4ee', '#242422'),
    noteTextColor: color('--accent-foreground', '#141413', '#f5f5f5'),
    noteBorderColor: color('--border', '#e5e3da', '#3a3a38'),
  }
}
