export type SearchEngine = 'duckduckgo' | 'google' | 'bing' | 'brave' | 'custom'

export const SEARCH_ENGINE_OPTIONS: ReadonlyArray<{ value: SearchEngine; label: string }> = [
  { value: 'duckduckgo', label: 'DuckDuckGo' },
  { value: 'google', label: 'Google' },
  { value: 'bing', label: 'Bing' },
  { value: 'brave', label: 'Brave Search' },
  { value: 'custom', label: 'Custom' },
]

const TEMPLATES: Record<Exclude<SearchEngine, 'custom'>, string> = {
  duckduckgo: 'https://duckduckgo.com/?q=%s',
  google: 'https://www.google.com/search?q=%s',
  bing: 'https://www.bing.com/search?q=%s',
  brave: 'https://search.brave.com/search?q=%s',
}

export function isUsableSearchTemplate(template: string): boolean {
  return /^https?:\/\/\S+$/i.test(template) && template.includes('%s')
}

/** The URL template (`%s` = the query) for the chosen engine; a custom template
 *  that is not an http(s) URL containing `%s` falls back to the default engine. */
export function searchTemplateFor(engine: SearchEngine, customTemplate: string): string {
  if (engine === 'custom' && isUsableSearchTemplate(customTemplate.trim())) {
    return customTemplate.trim()
  }
  return engine === 'custom' ? TEMPLATES.duckduckgo : TEMPLATES[engine]
}
