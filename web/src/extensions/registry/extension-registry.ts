// Stub: extensions feature is out of scope for this session.
// FUTURE: implement the full ExtensionRegistry with language, formatter, and linter support.

export interface FormatterConfig {
  command: string
  name: string
  args?: string[]
  env?: Record<string, string>
  inputMethod?: string
  outputMethod?: string
}

export interface LinterConfig {
  command: string
  name: string
  args?: string[]
  env?: Record<string, string>
}

export interface SnippetDefinition {
  prefix: string
  body: string | string[]
  description?: string
  scope?: string
}

export class ExtensionRegistry {
  getLanguageId(_filePath: string): string | null {
    return null
  }

  getFormatterForFile(_filePath: string): FormatterConfig | null {
    return null
  }

  getFormatterForLanguage(_languageId: string): FormatterConfig | null {
    return null
  }

  getLinterForFile(_filePath: string): LinterConfig | null {
    return null
  }

  getLinterForLanguage(_languageId: string): LinterConfig | null {
    return null
  }

  registerExtension(_extension: unknown): void {}

  isLspSupported(_filePath: string): boolean {
    return false
  }
  isLspSupportedForLanguage(_languageId: string): boolean {
    return false
  }

  getSnippetsForLanguage(_languageId: string): SnippetDefinition[] {
    return []
  }
  getAllSnippets(): SnippetDefinition[] {
    return []
  }
  getLspServerPath(_languageId: string): string | null {
    return null
  }

  onThemeChange(_handler: () => void): () => void {
    return () => {}
  }

  onRegistryChange(_handler: () => void): () => void {
    return () => {}
  }
}

export const extensionRegistry = new ExtensionRegistry()
