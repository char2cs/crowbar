import { normalizeBrowserUrl } from '@/features/browser/lib/normalize-url'
import { searchTemplateFor } from '@/features/browser/lib/search-engines'
import type { Settings } from '@/features/settings/types/settings'

export function browserHomepage(
  settings: Pick<Settings, 'browserHomepage' | 'browserSearchEngine' | 'browserCustomSearchUrl'>,
): string | undefined {
  const template = searchTemplateFor(settings.browserSearchEngine, settings.browserCustomSearchUrl)
  return normalizeBrowserUrl(settings.browserHomepage, template) ?? undefined
}
