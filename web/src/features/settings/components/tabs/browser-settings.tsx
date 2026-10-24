import { useEffect, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import NumberInput from '@/components/ui/number-input'
import { primitiveConfirm } from '@/components/ui/primitive-dialog-service'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  isUsableSearchTemplate,
  SEARCH_ENGINE_OPTIONS,
  type SearchEngine,
} from '@/features/browser/lib/search-engines'
import { BROWSER_ZOOM_MAX, BROWSER_ZOOM_MIN } from '@/features/settings/lib/settings-normalization'
import { getDefaultSetting, useSettingsStore } from '@/features/settings/store'
import { toast } from '@/features/window/stores/toast-store'
import { browserApi } from '@/lib/browser-bridge'
import Section, { SettingRow } from '../settings-section'
import { SETTINGS_CONTROL_WIDTHS } from '../settings-control-widths'

const CLEAR_UNAVAILABLE =
  'Not available on this system: browser tabs share their storage with the app itself, so clearing it would erase app data too.'

async function clearBrowsingData() {
  const confirmed = await primitiveConfirm(
    'This signs you out of every site and deletes cached files and site storage of all browser tabs. Open pages keep what they already loaded until they reload.',
    { title: 'Clear browsing data?', confirmLabel: 'Clear data' },
  )
  if (!confirmed) return
  try {
    await browserApi.clearData()
    toast.success('Browsing data cleared', 'Reload open pages to see the change.')
  } catch (error) {
    toast.error('Could not clear browsing data', String(error))
  }
}

export const BrowserSettings = () => {
  const settings = useSettingsStore((state) => state.settings)
  const updateSetting = useSettingsStore((state) => state.updateSetting)
  const [canClearData, setCanClearData] = useState(false)

  useEffect(() => {
    let live = true
    browserApi
      .capabilities()
      .then((caps) => live && setCanClearData(caps.clearData))
      .catch(() => live && setCanClearData(false))
    return () => {
      live = false
    }
  }, [])

  const customUrl = settings.browserCustomSearchUrl
  const customInvalid = customUrl.trim() !== '' && !isUsableSearchTemplate(customUrl.trim())

  return (
    <div className="space-y-4">
      <Section title="Navigation">
        <SettingRow
          label="Homepage"
          description="Page a new browser tab opens on; leave empty for a blank address bar"
          onReset={() => updateSetting('browserHomepage', getDefaultSetting('browserHomepage'))}
          canReset={settings.browserHomepage !== getDefaultSetting('browserHomepage')}
        >
          <Input
            size="sm"
            placeholder="https://example.com"
            value={settings.browserHomepage}
            onChange={(e) => updateSetting('browserHomepage', e.target.value)}
            className={SETTINGS_CONTROL_WIDTHS.textWide}
          />
        </SettingRow>

        <SettingRow
          label="Search Engine"
          description="Used when the address bar text is not a web address"
          onReset={() =>
            updateSetting('browserSearchEngine', getDefaultSetting('browserSearchEngine'))
          }
          canReset={settings.browserSearchEngine !== getDefaultSetting('browserSearchEngine')}
        >
          <Select
            value={settings.browserSearchEngine}
            onValueChange={(val) => {
              if (val) updateSetting('browserSearchEngine', val as SearchEngine)
            }}
          >
            <SelectTrigger size="sm" className={SETTINGS_CONTROL_WIDTHS.default}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {SEARCH_ENGINE_OPTIONS.map((opt) => (
                <SelectItem key={opt.value} value={opt.value}>
                  {opt.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </SettingRow>

        {settings.browserSearchEngine === 'custom' && (
          <SettingRow
            label="Custom Search URL"
            description={
              customInvalid
                ? 'Must be an http(s) address containing %s; DuckDuckGo is used until it is'
                : 'Search address for the custom engine, with %s where the query goes'
            }
            onReset={() =>
              updateSetting('browserCustomSearchUrl', getDefaultSetting('browserCustomSearchUrl'))
            }
            canReset={customUrl !== getDefaultSetting('browserCustomSearchUrl')}
          >
            <Input
              size="sm"
              placeholder="https://kagi.com/search?q=%s"
              value={customUrl}
              aria-invalid={customInvalid}
              onChange={(e) => updateSetting('browserCustomSearchUrl', e.target.value)}
              className={SETTINGS_CONTROL_WIDTHS.textWide}
            />
          </SettingRow>
        )}
      </Section>

      <Section title="Display">
        <SettingRow
          label="Page Zoom"
          description="Default zoom for pages, in percent; applies to tabs opened afterwards"
          onReset={() => updateSetting('browserZoom', getDefaultSetting('browserZoom'))}
          canReset={settings.browserZoom !== getDefaultSetting('browserZoom')}
        >
          <NumberInput
            min={String(BROWSER_ZOOM_MIN)}
            max={String(BROWSER_ZOOM_MAX)}
            step={10}
            value={settings.browserZoom}
            onChange={(val) => updateSetting('browserZoom', val)}
            className={SETTINGS_CONTROL_WIDTHS.number}
            size="xs"
          />
        </SettingRow>
      </Section>

      <Section title="Privacy">
        <SettingRow
          label="Clear Browsing Data"
          description={
            canClearData
              ? 'Delete cookies, cache and site storage of every browser tab'
              : CLEAR_UNAVAILABLE
          }
        >
          <Button
            variant="outline"
            size="sm"
            aria-label="Clear browsing data"
            disabled={!canClearData}
            onClick={() => void clearBrowsingData()}
          >
            Clear data
          </Button>
        </SettingRow>
      </Section>
    </div>
  )
}
