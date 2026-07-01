// Single approved entry point for the native OS file/folder picker.
// Components import from here instead of `@tauri-apps/plugin-dialog` directly
// so the eslint `no-restricted-imports` bridge-only rule has one place to allow.

export { open as openNativeDialog } from '@tauri-apps/plugin-dialog'
