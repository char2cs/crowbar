// Stub: extensions feature is out of scope for this session.
// FUTURE: implement the full ExtensionRegistry with language, formatter, and linter support.

export class ExtensionRegistry {
  getLanguageId(_filePath: string): string | null {
    return null
  }

  getFormatterForFile(_filePath: string): null {
    return null
  }

  getFormatterForLanguage(_languageId: string): null {
    return null
  }

  getLinterForFile(_filePath: string): null {
    return null
  }

  getLinterForLanguage(_languageId: string): null {
    return null
  }

  registerExtension(_extension: unknown): void {}

  isLspSupported(_filePath: string): boolean { return false }
  isLspSupportedForLanguage(_languageId: string): boolean { return false }

  getSnippetsForLanguage(_languageId: string): unknown[] { return [] }
  getLspServerPath(_languageId: string): string | null { return null }

  onThemeChange(_handler: () => void): () => void {
    return () => {}
  }

  onRegistryChange(_handler: () => void): () => void {
    return () => {}
  }
}

export const extensionRegistry = new ExtensionRegistry()
