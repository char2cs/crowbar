# Shell spec 00: design tokens (colour)

Source of truth: `app/panes/src/styles/{theme.css,crowbar.css,theme-vars.css,theme-fonts.css}`, `app/panes/src/index.css`. Line refs are `file:line` of the declaration. Part b (`00-tokens-b.md`) covers type, radii, shadows, motion, z-order, icons and the base primitives.
Hex and oklch values below are computed, not hand-copied: `scratchpad/tok/resolve.py` parses the CSS blocks, resolves `var()`, `color-mix(in oklch ...)`, `color(display-p3 ...)` and the Tailwind 4.3.3 palette, then converts to sRGB (alpha as `#rrggbbaa`). Re-run it after any token edit.

## 1. Themes and modes

| Axis | Values | Source |
|---|---|---|
| Theme id (`data-theme` on `<html>`) | `crowbar` (default), `frosted` | `extensions/themes/theme-registry.ts:11-26`; default `features/settings/config/default-settings.ts:56` |
| Mode (`.dark` class on `<html>`, `data-theme-type`) | `light`, `dark`, `system` (default `system`, follows `prefers-color-scheme`, falls back to dark when unreadable) | `default-settings.ts:58`, `settings-effects.ts:34-55`, `settings-bootstrap.ts:13` |
| Platform (`data-platform` on `<html>`) | `linux`, `macos`, `windows`, `ios` from the user agent; macOS has window vibrancy, Linux and iOS do not | `utils/platform.ts:5-27`, `appearance-bootstrap.ts:137`, `theme.css:307-321` |
| First paint before settings load | cache `crowbar.bootstrap.appearance.v2`; default `{themeId:'crowbar', themeType:'dark'}` | `appearance-bootstrap.ts:32,59-68` |

Facts the native shell must reproduce:

1. There is no CSS block for `frosted`: it is the unthemed base (`theme.css:3`, `:root` + `.dark`). `crowbar` is the base plus the overrides in `crowbar.css`. So only two real palettes exist: base (frosted) and crowbar, each in light and dark.
2. Crowbar paints its chrome dark in light mode too. Two regions are `.chrome-surface` and take the full `.dark` palette while the rest of the app is light: the sidebar content (`components/layout/ide-shell.tsx:85`) and the view-tabs row (`features/tabs/components/view-tabs-row.tsx:73`). Selector list at `theme.css:167-169`, extra overrides at `crowbar.css:25-32`. Column "chrome-surface (light)" below is that cascade.
3. Raised controls (`control-surface`: selected pill, active tab) are `#fcfcfa` with `#1b1b19` text in BOTH modes under crowbar (`crowbar.css:9-14`). Under frosted they follow `--background/--foreground` (`theme.css:76-79`).
4. Reference environment is Linux (`data-platform="linux"`): `--chrome-bg` and `--pane-chrome-bg` are opaque there (`theme.css:310-321`, `crowbar.css:36-39`). macOS values are translucent (section 5). Windows is out of scope for the product.
5. Cascade winners: `[data-theme='crowbar'][data-platform='linux']:not(.dark)` (0,3,0) ties `[data-theme='crowbar']:not(.dark):not([data-platform='ios'])` (0,3,0) and wins by order (`crowbar.css:28-39`). Dark: `.dark[data-platform='linux']` (0,2,0) beats `.dark` (0,1,0) (`theme.css:316`).
6. A lifted drag surface (`[data-drag-ghost]`, `[data-drag-overlay]`) always takes the LIGHT palette in any mode (`theme.css:5-8`).
7. Tailwind `border-border outline-ring/50` is applied to every element (`index.css:~75`, `@layer base *`): default border colour is `--border`, default outline colour is `--ring` at 50%.
8. Body background is `bg-chrome-bg`, body text `text-foreground`; `html, body, #root` are `height:100%; overflow:hidden` (`index.css` `@layer base`).

## 2. Colour tokens, hex (Linux, sRGB, alpha as trailing byte)

Columns: crowbar light and dark are the `<html>` values. "chrome-surface (light)" is the value inside the sidebar and view-tabs row when crowbar is in light mode. Frosted columns are the unthemed base.

| token | crowbar light | crowbar dark | chrome-surface (light) | frosted light | frosted dark |
|---|---|---|---|---|---|
| **Surfaces** | | | | | |
| `--background` | `#fcfcfa` | `#1c1c1a` | `#1c1c1a` | `#fcfcfa` | `#1c1c1a` |
| `--pane-background` | `#fcfcfa` | `#1c1c1a` | `#1c1c1a` | `#fcfcfa` | `#1c1c1a` |
| `--pane-chrome-bg` | `#fcfcfa` | `#121210` | `#10100ea6` | `#fafafa` | `#121210` |
| `--chrome-bg` | `#121210` | `#121210` | `#10100ea6` | `#fbfbfb` | `#121210` |
| `--background-inverse` | `#1c1c1a` | `#fcfcfa` | `#fcfcfa` | `#1c1c1a` | `#fcfcfa` |
| `--foreground` | `#1b1b19` | `#f2f0ea` | `#f2f0ea` | `#1b1b19` | `#f2f0ea` |
| `--foreground-inverse` | `#f2f0ea` | `#1b1b19` | `#1b1b19` | `#f2f0ea` | `#1b1b19` |
| `--card` | `#ffffff` | `#1c1c1a` | `#1c1c1a` | `#ffffff` | `#1c1c1a` |
| `--card-foreground` | `#1b1b19` | `#f2f0ea` | `#f2f0ea` | `#1b1b19` | `#f2f0ea` |
| `--popover` | `#ffffff` | `#1c1c1a` | `#1c1c1a` | `#ffffff` | `#1c1c1a` |
| `--popover-foreground` | `#1b1b19` | `#f2f0ea` | `#f2f0ea` | `#1b1b19` | `#f2f0ea` |
| `--muted` | `#0000000a` | `#262624` | `#262624` | `#0000000a` | `#262624` |
| `--muted-foreground` | `#484848` | `#c7c8ce` | `#c7c8ce` | `#484848` | `#c7c8ce` |
| `--accent` | `#00000014` | `#ffffff1a` | `#ffffff1a` | `#00000014` | `#ffffff1a` |
| `--accent-foreground` | `#1b1b19` | `#f2f0ea` | `#f2f0ea` | `#1b1b19` | `#f2f0ea` |
| `--border` | `#dddcd6` | `#333330` | `#333330` | `#dddcd6` | `#333330` |
| `--input` | `#dddcd6` | `#333330` | `#333330` | `#dddcd6` | `#333330` |
| `--ring` | `#a1a1a1` | `#737373` | `#737373` | `#a1a1a1` | `#737373` |
| **Brand / semantic** | | | | | |
| `--primary` | `#606c38` | `#98a86a` | `#98a86a` | `#606c38` | `#98a86a` |
| `--primary-foreground` | `#ffffff` | `#1b1b19` | `#1b1b19` | `#ffffff` | `#1b1b19` |
| `--secondary` | `#606c38` | `#98a86a` | `#98a86a` | `#606c38` | `#98a86a` |
| `--secondary-foreground` | `#ffffff` | `#1b1b19` | `#1b1b19` | `#ffffff` | `#1b1b19` |
| `--brand` | `#606c38` | `#98a86a` | `#98a86a` | `#606c38` | `#98a86a` |
| `--brand-light` | `#606c38` | `#606c38` | `#606c38` | `#606c38` | `#606c38` |
| `--brand-dark` | `#98a86a` | `#98a86a` | `#98a86a` | `#98a86a` | `#98a86a` |
| `--brand-ground` | `#98a86a` | `#243713` | `#243713` | `#98a86a` | `#243713` |
| `--destructive` | `#b8422c` | `#f1a493` | `#f1a493` | `#b8422c` | `#f1a493` |
| `--destructive-foreground` | `#7c2717` | `#3b211c` | `#3b211c` | `#7c2717` | `#3b211c` |
| `--success` | `#4e7a36` | `#b9cf98` | `#b9cf98` | `#4e7a36` | `#b9cf98` |
| `--success-foreground` | `#2e5122` | `#2a2f1f` | `#2a2f1f` | `#2e5122` | `#2a2f1f` |
| `--warning` | `#d9a441` | `#e9c97f` | `#e9c97f` | `#d9a441` | `#e9c97f` |
| `--warning-foreground` | `#634609` | `#372c17` | `#372c17` | `#634609` | `#372c17` |
| `--info` | `#527ca8` | `#7aa6d8` | `#7aa6d8` | `#527ca8` | `#7aa6d8` |
| `--info-foreground` | `#1447e6` | `#51a2ff` | `#51a2ff` | `#1447e6` | `#51a2ff` |
| **Control surface (raised pill / active tab)** | | | | | |
| `--control-surface` | `#fcfcfa` | `#fcfcfa` | `#fcfcfa` | `#fcfcfa` | `#1c1c1a` |
| `--control-foreground` | `#1b1b19` | `#1b1b19` | `#1b1b19` | `#1b1b19` | `#f2f0ea` |
| `--control-muted-foreground` | `#484848` | `#484848` | `#484848` | `#484848` | `#c7c8ce` |
| `--control-highlight` | `#1b1b1929` | `#1b1b1929` | `#1b1b1929` | `#1b1b1929` | `#f2f0ea29` |
| `--elevated-highlight` | `#1b1b1929` | `#f2f0ea29` | `#f2f0ea29` | `#1b1b1929` | `#f2f0ea29` |
| **Pane frame** | | | | | |
| `--pane-accent` | `#606c38` | `#98a86a` | `#98a86a` | `#606c38` | `#98a86a` |
| `--pane-border` | `#717171` | `#333330` | `#717171` | `#dddcd6` | `#333330` |
| `--editor-selection` | `#606c3833` | `#98a86a52` | `#98a86a52` | `#606c3833` | `#98a86a52` |
| **Sidebar** | | | | | |
| `--sidebar` | `#0000002e` | `#0000002e` | `#0000002e` | `#00000000` | `#0000002e` |
| `--sidebar-foreground` | `#1b1b19` | `#f2f0ea` | `#f2f0ea` | `#1b1b19` | `#f2f0ea` |
| `--sidebar-primary` | `#1b1b19` | `#f2f0ea` | `#f2f0ea` | `#1b1b19` | `#f2f0ea` |
| `--sidebar-primary-foreground` | `#fcfcfa` | `#1c1c1a` | `#1c1c1a` | `#fcfcfa` | `#1c1c1a` |
| `--sidebar-accent` | `#00000014` | `#ffffff1a` | `#ffffff1a` | `#00000014` | `#ffffff1a` |
| `--sidebar-accent-foreground` | `#1b1b19` | `#f2f0ea` | `#f2f0ea` | `#1b1b19` | `#f2f0ea` |
| `--sidebar-border` | `#dddcd6` | `#333330` | `#333330` | `#dddcd6` | `#333330` |
| `--sidebar-ring` | `#a1a1a1` | `#a1a1a1` | `#a1a1a1` | `#a1a1a1` | `#a1a1a1` |
| `--sidebar-element-idle` | `#1b1b190f` | `#f2f0ea1c` | `#f2f0ea1c` | `#1b1b190f` | `#f2f0ea1c` |
| `--sidebar-element-hover` | `#00000014` | `#ffffff1a` | `#ffffff1a` | `#00000014` | `#ffffff1a` |
| `--sidebar-element-press` | `#1b1b1914` | `#f2f0ea12` | `#f2f0ea12` | `#1b1b1914` | `#f2f0ea12` |
| `--sidebar-count-added` | `#4e7a36` | `#b9cf98` | `#b9cf98` | `#4e7a36` | `#b9cf98` |
| `--sidebar-count-deleted` | `#b8422c` | `#f1a493` | `#f1a493` | `#b8422c` | `#f1a493` |
| `--sidebar-drop-line` | `#424529` | `#c6c6a0` | `#c6c6a0` | `#424529` | `#c6c6a0` |
| `--sidebar-drop-nest` | `#606c384c` | `#98a86a4c` | `#98a86a4c` | `#606c384c` | `#98a86a4c` |
| `--sidebar-drop-nest-edge` | `#606c38a6` | `#98a86aa6` | `#98a86aa6` | `#606c38a6` | `#98a86aa6` |
| **Git status** | | | | | |
| `--git-added` | `#4e7a36` | `#80a36f` | `#80a36f` | `#4e7a36` | `#80a36f` |
| `--git-deleted` | `#b8422c` | `#e0795f` | `#e0795f` | `#b8422c` | `#e0795f` |
| `--git-modified` | `#d9a441` | `#d5a24a` | `#d5a24a` | `#d9a441` | `#d5a24a` |
| `--git-modified-staged` | `#4e7a36` | `#deb15d` | `#deb15d` | `#4e7a36` | `#deb15d` |
| `--git-untracked` | `#5ea500` | `#7aa6d8` | `#7aa6d8` | `#5ea500` | `#7aa6d8` |
| `--git-renamed` | `#0084d1` | `#b693ce` | `#b693ce` | `#0084d1` | `#b693ce` |
| **Code / chart** | | | | | |
| `--code` | `#ffffff` | `#1c1c1a` | `#1c1c1a` | `#ffffff` | `#1c1c1a` |
| `--code-foreground` | `#1b1b19` | `#f2f0ea` | `#f2f0ea` | `#1b1b19` | `#f2f0ea` |
| `--code-highlight` | `#0000000a` | `#ffffff0a` | `#ffffff0a` | `#0000000a` | `#ffffff0a` |
| `--chart-1` | `#f54900` | `#1447e6` | `#1447e6` | `#f54900` | `#1447e6` |
| `--chart-2` | `#009689` | `#00bc7d` | `#00bc7d` | `#009689` | `#00bc7d` |
| `--chart-3` | `#104e64` | `#fe9a00` | `#fe9a00` | `#104e64` | `#fe9a00` |
| `--chart-4` | `#ffb900` | `#ad46ff` | `#ad46ff` | `#ffb900` | `#ad46ff` |
| `--chart-5` | `#fe9a00` | `#ff2056` | `#ff2056` | `#fe9a00` | `#ff2056` |

## 3. Colour tokens, oklch (crowbar, as authored then resolved)

|---|---|---|
| `--background` | `oklch(0.9905 0.00263 106.4)` | `oklch(0.2257 0.00379 106.7)` |
| `--pane-background` | `oklch(0.9905 0.00263 106.4)` | `oklch(0.2257 0.00379 106.7)` |
| `--pane-chrome-bg` | `oklch(0.9905 0.00263 106.4)` | `oklch(0.182 0.005 106.7)` |
| `--chrome-bg` | `oklch(0.182 0.005 106.7)` | `oklch(0.182 0.005 106.7)` |
| `--background-inverse` | `oklch(0.2257 0.00379 106.7)` | `oklch(0.9905 0.00263 106.4)` |
| `--foreground` | `oklch(0.2213 0.00381 106.7)` | `oklch(0.955 0.00826 91.48)` |
| `--foreground-inverse` | `oklch(0.955 0.00826 91.48)` | `oklch(0.2213 0.00381 106.7)` |
| `--card` | `oklch(1 3.73e-08 89.88)` | `oklch(0.2257 0.00379 106.7)` |
| `--card-foreground` | `oklch(0.2213 0.00381 106.7)` | `oklch(0.955 0.00826 91.48)` |
| `--popover` | `oklch(1 3.73e-08 89.88)` | `oklch(0.2257 0.00379 106.7)` |
| `--popover-foreground` | `oklch(0.2213 0.00381 106.7)` | `oklch(0.955 0.00826 91.48)` |
| `--muted` | `oklch(0 0 0 / 0.04)` | `oklch(0.2679 0.00364 106.6)` |
| `--muted-foreground` | `oklch(0.4 0 0)` | `oklch(0.834 0.008 278.6)` |
| `--accent` | `oklch(0 0 0 / 0.08)` | `oklch(1 0 0 / 0.1)` |
| `--accent-foreground` | `oklch(0.2213 0.00381 106.7)` | `oklch(0.955 0.00826 91.48)` |
| `--border` | `oklch(0.8936 0.00817 98.89)` | `oklch(0.32 0.00522 106.7)` |
| `--input` | `oklch(0.8936 0.00817 98.89)` | `oklch(0.32 0.00522 106.7)` |
| `--ring` | `oklch(0.708 0 0)` | `oklch(0.556 0 0)` |
| `--primary` | `oklch(0.5082 0.0762 119.9)` | `oklch(0.7039 0.0864 120.4)` |
| `--primary-foreground` | `oklch(1 3.73e-08 89.88)` | `oklch(0.2213 0.00381 106.7)` |
| `--secondary` | `oklch(0.5082 0.0762 119.9)` | `oklch(0.7039 0.0864 120.4)` |
| `--secondary-foreground` | `oklch(1 3.73e-08 89.88)` | `oklch(0.2213 0.00381 106.7)` |
| `--brand` | `oklch(0.5082 0.0762 119.9)` | `oklch(0.7039 0.0864 120.4)` |
| `--brand-light` | `oklch(0.5082 0.0762 119.9)` | `oklch(0.5082 0.0762 119.9)` |
| `--brand-dark` | `oklch(0.7039 0.0864 120.4)` | `oklch(0.7039 0.0864 120.4)` |
| `--brand-ground` | `oklch(0.7039 0.0864 120.4)` | `oklch(0.3094 0.0618 132.1)` |
| `--destructive` | `oklch(0.5403 0.157 32.67)` | `oklch(0.7895 0.0953 32.96)` |
| `--destructive-foreground` | `oklch(0.4008 0.121 32.78)` | `oklch(0.2804 0.0416 31.58)` |
| `--success` | `oklch(0.531 0.109 135.7)` | `oklch(0.8241 0.0777 125.8)` |
| `--success-foreground` | `oklch(0.3959 0.0843 138.3)` | `oklch(0.2951 0.0285 122.2)` |
| `--warning` | `oklch(0.7507 0.13 79.85)` | `oklch(0.8471 0.0997 86.92)` |
| `--warning-foreground` | `oklch(0.4157 0.0816 79.83)` | `oklch(0.2997 0.0375 82.61)` |
| `--info` | `oklch(0.574 0.083 250.5)` | `oklch(0.713 0.088 252.2)` |
| `--info-foreground` | `oklch(0.488 0.243 264.4)` | `oklch(0.707 0.165 254.6)` |
| `--control-surface` | `oklch(0.9905 0.00263 106.4)` | `oklch(0.9905 0.00263 106.4)` |
| `--control-foreground` | `oklch(0.2213 0.00381 106.7)` | `oklch(0.2213 0.00381 106.7)` |
| `--control-muted-foreground` | `oklch(0.4 0 0)` | `oklch(0.4 0 0)` |
| `--control-highlight` | `oklch(0.2213 0.00381 106.7 / 0.16)` | `oklch(0.2213 0.00381 106.7 / 0.16)` |
| `--elevated-highlight` | `oklch(0.2213 0.00381 106.7 / 0.16)` | `oklch(0.955 0.00826 91.48 / 0.16)` |
| `--pane-accent` | `oklch(0.5082 0.0762 119.9)` | `oklch(0.7039 0.0864 120.4)` |
| `--pane-border` | `oklch(0.55 0 0)` | `oklch(0.32 0.00522 106.7)` |
| `--editor-selection` | `oklch(0.5082 0.0762 119.9 / 0.2)` | `oklch(0.7039 0.0864 120.4 / 0.32)` |
| `--sidebar` | `oklch(0 0 0 / 0.18)` | `oklch(0 0 0 / 0.18)` |
| `--sidebar-foreground` | `oklch(0.2213 0.00381 106.7)` | `oklch(0.955 0.00826 91.48)` |
| `--sidebar-primary` | `oklch(0.2213 0.00381 106.7)` | `oklch(0.955 0.00826 91.48)` |
| `--sidebar-primary-foreground` | `oklch(0.9905 0.00263 106.4)` | `oklch(0.2257 0.00379 106.7)` |
| `--sidebar-accent` | `oklch(0 0 0 / 0.08)` | `oklch(1 0 0 / 0.1)` |
| `--sidebar-accent-foreground` | `oklch(0.2213 0.00381 106.7)` | `oklch(0.955 0.00826 91.48)` |
| `--sidebar-border` | `oklch(0.8936 0.00817 98.89)` | `oklch(0.32 0.00522 106.7)` |
| `--sidebar-ring` | `oklch(0.708 0 0)` | `oklch(0.708 0 0)` |
| `--sidebar-element-idle` | `oklch(0.2213 0.00381 106.7 / 0.06)` | `oklch(0.955 0.00826 91.48 / 0.11)` |
| `--sidebar-element-hover` | `oklch(0 0 0 / 0.08)` | `oklch(1 0 0 / 0.1)` |
| `--sidebar-element-press` | `oklch(0.2213 0.00381 106.7 / 0.08)` | `oklch(0.955 0.00826 91.48 / 0.07)` |
| `--sidebar-count-added` | `oklch(0.531 0.109 135.7)` | `oklch(0.8241 0.0777 125.8)` |
| `--sidebar-count-deleted` | `oklch(0.5403 0.157 32.67)` | `oklch(0.7895 0.0953 32.96)` |
| `--sidebar-drop-line` | `oklch(0.3791 0.0436 113.9)` | `oklch(0.8169 0.0512 107.4)` |
| `--sidebar-drop-nest` | `oklch(0.5082 0.0762 119.9 / 0.3)` | `oklch(0.7039 0.0864 120.4 / 0.3)` |
| `--sidebar-drop-nest-edge` | `oklch(0.5082 0.0762 119.9 / 0.65)` | `oklch(0.7039 0.0864 120.4 / 0.65)` |
| `--git-added` | `oklch(0.531 0.109 135.7)` | `oklch(0.676 0.083 135.7)` |
| `--git-deleted` | `oklch(0.5403 0.157 32.67)` | `oklch(0.686 0.134 35.2)` |
| `--git-modified` | `oklch(0.7507 0.13 79.85)` | `oklch(0.743 0.121 78.6)` |
| `--git-modified-staged` | `oklch(0.531 0.109 135.7)` | `oklch(0.784 0.115 81.4)` |
| `--git-untracked` | `oklch(0.648 0.2 131.7)` | `oklch(0.713 0.088 252.2)` |
| `--git-renamed` | `oklch(0.588 0.158 242)` | `oklch(0.716 0.092 311.1)` |
| `--code` | `oklch(1 3.73e-08 89.88)` | `oklch(0.2257 0.00379 106.7)` |
| `--code-foreground` | `oklch(0.2213 0.00381 106.7)` | `oklch(0.955 0.00826 91.48)` |
| `--code-highlight` | `oklch(0 0 0 / 0.04)` | `oklch(1 0 0 / 0.04)` |
| `--chart-1` | `oklch(0.646 0.222 41.12)` | `oklch(0.488 0.243 264.4)` |
| `--chart-2` | `oklch(0.6 0.118 184.7)` | `oklch(0.696 0.17 162.5)` |
| `--chart-3` | `oklch(0.398 0.07 227.4)` | `oklch(0.769 0.188 70.08)` |
| `--chart-4` | `oklch(0.828 0.189 84.43)` | `oklch(0.627 0.265 303.9)` |
| `--chart-5` | `oklch(0.769 0.188 70.08)` | `oklch(0.645 0.246 16.44)` |

Authored literals (not derived): `--background #fcfcfa` / `#1c1c1a` (`theme.css:10,170`), `--foreground #1b1b19` / `#f2f0ea` (`:12,172`), `--primary #606c38` / `#98a86a` (`:21,182`), `--border #dddcd6` / `#333330` (`:51,210`), `--muted` dark `#262624` (`:190`), `--destructive #b8422c` / `#f1a493` (`:43,202`), `--success #4e7a36` / `#b9cf98` (`:47,206`), `--warning #d9a441` / `#e9c97f` (`:49,208`). Everything else derives via `color-mix(in oklch, ...)` or oklch literals as in the source. `--ring` is Tailwind neutral-400 (`#a1a1a1`) light and neutral-500 (`#737373`) dark (`theme.css:53,212`). `--info-foreground` is blue-700 light and blue-400 dark. `--pane-accent-width` is 2px in crowbar light, 1px otherwise (`crowbar.css:42`, `theme.css:74`). `--brand-ground` dark is `color(display-p3 0.15686 0.21176 0.09412)` (`theme.css:201`), shown as sRGB `#243713`.

## 4. Derived formulas worth knowing (native code should compute, not hard-code)

| Token | Formula | Source |
|---|---|---|
| `--sidebar-element-idle` | `foreground` 6% (light) / 11% (dark) over transparent | `theme.css:67,226` |
| `--sidebar-element-hover` | black 8% (light), white 10% (dark) | `theme.css:68,227` |
| `--sidebar-element-press` | `foreground` 8% (light) / 7% (dark) | `theme.css:69,228` |
| `--elevated-highlight` | `foreground` 16% | `theme.css:70`; crowbar control version `crowbar.css:13` uses `#1b1b19` 16% |
| `--sidebar-drop-line` | `mix(secondary 55%, foreground)` | `theme.css:95` |
| `--sidebar-drop-nest` / `-edge` | `secondary` 30% / 65% over transparent | `theme.css:96-97` |
| `--accent` | black 8% (light), white 10% (dark); menu row highlight | `theme.css:32,196` |
| `--muted` | black 4% (light), `#262624` solid (dark) | `theme.css:26,190` |
| `--editor-selection` | primary 20% (light) / 32% (dark) | `theme.css:35,198` |

## 5. macOS-only translucent values (CI runner reference; not on Linux)

| token | light | dark | source |
|---|---|---|---|
| `--chrome-bg` | `#10100ea6` oklch(0.173 0.004 106.7 / 0.65) | same | `crowbar.css:20` / `theme.css:245` |
| `--pane-chrome-bg` | `#fcfcfaf2` = pane-background 95% | `#10100ea6` | `crowbar.css:21`, `theme.css:248` |
| `--sidebar` | `#0000002e` black 18% | same | `crowbar.css:19`, `theme.css:218` |

## 6. Terminal ANSI palette (crowbar; light uses `--destructive`, `--success`, etc.)

| ansi | crowbar light | crowbar dark |
| black | `#262626` | `#080809` |
| red | `#b8422c` | `#e0795f` |
| green | `#4e7a36` | `#80a36f` |
| yellow | `#d9a441` | `#d5a24a` |
| blue | `#527ca8` | `#7aa6d8` |
| magenta | `#5b754a` | `#b693ce` |
| cyan | `#b65b46` | `#88b5c6` |
| white | `#484848` | `#c7c8ce` |
| bright-black | `#484848` | `#757982` |
| bright-red | `#b8422c` | `#ea9177` |
| bright-green | `#4e7a36` | `#93b584` |
| bright-yellow | `#d9a441` | `#e0b65f` |
| bright-blue | `#527ca8` | `#93bde9` |
| bright-magenta | `#5b754a` | `#c8a7db` |
| bright-cyan | `#b65b46` | `#9ec9d7` |
| bright-white | `#1b1b19` | `#f3f4f6` |

Source `theme.css:147-162` (light), `:278-293` (dark).

## 7. Repo avatar tile colours

Tailwind 700 of eight names; class emitted for each token the daemon sends (`index.css:~88-125`, `@source inline`). Letter glyph is on top, text colour per `repo-avatar.tsx`.

| token | hex | oklch |
|---|---|---|
| avatar-rose | `#c70036` | `oklch(51.4% 0.222 16.935) ` |
| avatar-amber | `#bb4d00` | `oklch(55.5% 0.163 48.998) ` |
| avatar-emerald | `#007a55` | `oklch(50.8% 0.118 165.612) ` |
| avatar-cyan | `#007595` | `oklch(52% 0.105 223.128) ` |
| avatar-indigo | `#432dd7` | `oklch(45.7% 0.24 277.023) ` |
| avatar-violet | `#7008e7` | `oklch(49.1% 0.27 292.581) ` |
| avatar-slate | `#314158` | `oklch(37.2% 0.044 257.287) ` |
| avatar-pink | `#c6005c` | `oklch(52.5% 0.223 3.958) ` |

Other Tailwind colours referenced by shell surfaces: red-500 `#fb2c36`, green-500 `#00c950`, violet-500 `#8e51ff`, amber-500 `#fe9a00` (`workspace-branch-icon.tsx:50-72`), neutral-400 `#a1a1a1`, neutral-500 `#737373`, white `#ffffff`, black `#000000`.
