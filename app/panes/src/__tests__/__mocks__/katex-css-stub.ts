// Stub for `katex/dist/katex.min.css` in the test environment.
//
// Both this app's own math-kit CSS import (markdown-plugins.ts) and
// @platejs/math's own internal side-effect import of the same stylesheet
// resolve to this specifier text. Vitest externalizes node_modules imports
// by default (bypassing Vite's CSS-to-JS transform for perf), so the raw
// `.css` file hits Node's ESM loader directly and fails with "Unknown file
// extension \".css\"" — same fix shape as the `@tauri-apps/api/core` stub
// beside this file: alias the specifier to an inert module for tests, which
// don't render anything that needs the stylesheet's actual rules.
export {}
