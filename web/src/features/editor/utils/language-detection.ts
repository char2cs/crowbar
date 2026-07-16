import { extensionRegistry } from '@/extensions/registry/extension-registry'
import {
  ANGULAR_TEMPLATE_LANGUAGE_ID,
  isAngularTemplatePath,
} from '@/features/editor/lib/wasm-parser/language-overlays'
import { extensionManager } from '../extensions/manager'

/**
 * Detect programming language from file extension using the extension registry
 */
export function detectLanguageFromPath(filePath: string): string {
  if (isAngularTemplatePath(filePath)) {
    return ANGULAR_TEMPLATE_LANGUAGE_ID
  }

  const fromRegistry = extensionRegistry.getLanguageId(filePath)
  if (fromRegistry) {
    return fromRegistry
  }

  const extension = filePath.toLowerCase().split('.').pop() || ''

  // First, try to get language from extension manager
  const languageProvider = extensionManager.getLanguageProvider(extension)
  if (languageProvider) {
    return languageProvider.id
  }

  // Fallback to static map for unsupported languages
  const languageMap: Record<string, string> = {
    // Unsupported languages that might be added in the future
    tsx: 'typescriptreact',
    dockerignore: 'gitignore',
    gitattributes: 'gitattributes',
    gitignore: 'gitignore',
    ignore: 'gitignore',
    lock: 'lockfile',
    zig: 'zig',
    el: 'elisp',
    scss: 'scss',
    sass: 'sass',
    less: 'less',
    xml: 'xml',
    svg: 'xml',
    rst: 'restructuredtext',
    tex: 'latex',
    scala: 'scala',
    hs: 'haskell',
    ml: 'ocaml',
    fs: 'fsharp',
    clj: 'clojure',
    lisp: 'lisp',
    scm: 'scheme',
    fish: 'shell',
    ps1: 'powershell',
    bat: 'batch',
    cmd: 'batch',
    ini: 'ini',
    cfg: 'ini',
    conf: 'ini',
    csv: 'csv',
    dockerfile: 'dockerfile',
    makefile: 'makefile',
    r: 'r',
    lua: 'lua',
    vim: 'vim',
    elm: 'elm',
  }

  return languageMap[extension] || 'text'
}
