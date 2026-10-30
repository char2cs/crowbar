// The phone's native chrome (SwiftUI over the webview, desktop/ios-chrome). The web owns every
// piece of state and hands the plugin a whole view-model; the plugin renders it and reports
// what the user asked for as intents. Anywhere else both calls do nothing.

import { addPluginListener, invoke, type PluginListener } from '@tauri-apps/api/core'
import { isTauri } from '@/lib/crowbar-bridge'
import { IS_MOBILE } from '@/utils/platform'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import type { AllowedModes, DropMode } from '@/components/tree-dnd/drop-core'
import type { HostDotState } from '@/features/hosts/types'
import type { HostNetState, NetState, ReadoutLines } from '@/components/layout/build-readout-lines'
import type { Project } from '@/lib/types'
import type { AgentProvider, PermissionLevel } from '@/features/agent/api/agent-api'
import type { AgentProvidersStatus } from '@/features/settings/stores/agent-providers-store'
import type { ChatHeaderModel } from '@/features/tabs/hooks/use-chat-branch-header-model'
import type { FileTreeGitStatusDecoration } from '@/features/file-explorer/lib/file-tree-git-status'

const PLUGIN = 'crowbar-chrome'

interface ConnectSheet {
  open: boolean
  stage: 'address' | 'fingerprint' | 'code'
  name: string
  address: string
  code: string
  fingerprint: string
  busy: boolean
  error: string | null
}

/** A round glass button in the top corners; `symbol` is an SF Symbol name. */
export interface ChromeButton {
  id: string
  symbol: string
  side: 'leading' | 'trailing'
  label: string
  enabled: boolean
  /** Sent as `{ type: intent }` instead of `tap`, for a button another listener handles. */
  intent?: string
}

export interface ChromeHeaders {
  chat: ChromeButton[]
  ide: ChromeButton[]
}

/** Which layer the shell's front card is slid aside to reveal. */
export type ChromeReveal = 'none' | 'chatList' | 'ide' | 'files'

export interface NativeChrome {
  /** The brand colour as CSS gives it (`#rrggbb`), for the native prominent buttons. */
  accent: string
  welcome: { visible: boolean; label: string }
  /** `--brand-ground` as CSS gives it for the current theme: the New thread pill's fill. */
  brandGround: string
  connect: ConnectSheet
  /** Each screen's own header buttons, always both: a header stays with its screen as it slides. */
  headers: ChromeHeaders
  reveal: ChromeReveal
  /** The newest navigation intent (its `seq`) this model already reflects. */
  ack: number
  /** The app's theme mode; `system` lets native views follow the device like the web does. */
  appearance: 'light' | 'dark' | 'system'
  /** The Settings sheet, whose one page is the web's own settings at `#/mobile-settings`. */
  settings: { open: boolean }
  /** The theme's colours for the native sheets, so they read as the web's own pages. */
  palette: NativePalette
}

/** Theme tokens as CSS gives them: `#hex`, `rgb()`, `color(display-p3 …)` or `oklch()`. */
export interface NativePalette {
  surface: string
  /** Cards and fields, one step lifted from `surface`. */
  raised: string
  text: string
  secondaryText: string
}

/** The chat list's whole content, replaced on every push. Swift forwards it untouched into
 *  the sidebar page (`#/mobile-sidebar`), which draws it with the desktop tree's own rows. */
// A type, not an interface: invoke's args need the index signature only a type alias gets.
export type NativeSidebar = {
  focusedChatId: string | null
  activeProjectId: string | null
  projects: Project[]
  hosts: { id: string; name: string; state: HostDotState }[]
  /** The active project's rows, flat and parentId-linked, live facts resolved. */
  rows: SidebarRow[]
  /** The rows above that have an agent running, pinned at the top: a view over `rows`. */
  active: { id: string }[]
  collapsed: string[]
  /** Each row's menu, by row id, for the native long-press menu. */
  actions: Record<string, NativeRowAction[]>
  /** While a row is dragged: where the desktop's drop rules let it land, by target row id. */
  drop: NativeRowDrop | null
  /** The build/net readout, drawn at the page's foot; null while it is turned off. */
  readout: NativeReadout | null
}

/** What the readout says, already resolved: the page watches none of its stores. */
export type NativeReadout = {
  lines: ReadoutLines
  net: NetState
  hosts: { name: string; net: HostNetState }[]
}

export type NativeRowDrop = {
  subjectId: string
  targets: Record<string, { modes: AllowedModes; band: number }>
}

/** One verb of a row's native menu; `confirm` asks natively before it is sent. */
export type NativeRowAction = {
  id: string
  title: string
  symbol: string
  destructive: boolean
  confirm: { title: string; message: string } | null
}

/** Settings → Hosts: every paired daemon, as the main webview holds them. */
// A type, not an interface, for invoke's args (see NativeSidebar).
export type NativeHostsSettings = {
  hosts: { id: string; name: string; endpoint: string; state: HostDotState }[]
  revoking: string[]
  /** The host whose projects the chat list shows, null while there is no other to choose. */
  shownHost: string | null
}

/** The host picker sheet: the same hosts as Settings → Hosts, listed in a model-picker-style sheet. */
// A type, not an interface, for invoke's args (see NativeSidebar).
export type NativeHostPicker = NativeHostsSettings & { open: boolean }

/** The Agents settings page's content: one host's agents, as the main webview holds them. */
// A type, not an interface, for invoke's args (see NativeSidebar).
export type NativeAgentsSettings = {
  hostId: string
  hostName: string
  status: AgentProvidersStatus
  providers: AgentProvider[]
  permissionLevel: PermissionLevel
}

/** One loaded entry of the workspace's tree, already filtered by the explorer's settings. */
export type NativeFilesEntry = {
  name: string
  path: string
  isDir: boolean
  isSymlink?: boolean
  ignored?: boolean
  git: FileTreeGitStatusDecoration | null
  children?: NativeFilesEntry[]
}

/** The IDE's file drawer (`#/mobile-files`): the focused workspace's tree as the main webview
 *  holds it. The page owns only its search box. */
// A type, not an interface, for invoke's args (see NativeSidebar).
export type NativeFiles = {
  /** The chat the files belong to, drawn as the desktop's chat header; null with no chat. */
  header: ChatHeaderModel | null
  loading: boolean
  files: NativeFilesEntry[]
  expanded: string[]
  activePath: string | null
  compactFolders: boolean
}

/** The IDE's native tab strip: the focused pane's editor tabs, left to right. Swift picks each
 *  tab's symbol from `kind`, and from `path`'s extension for a file. */
// A type, not an interface, for invoke's args (see NativeSidebar).
export type NativeTabs = {
  tabs: {
    id: string
    title: string
    kind: 'file' | 'preview' | 'terminal' | 'review' | 'diff' | 'browser' | 'newTab'
    path: string | null
    active: boolean
    dirty: boolean
    closable: boolean
  }[]
  /** The + menu's choices, sent back as `newTab`'s `actionId`. */
  newTabActions: { id: string; title: string; symbol: string }[]
}

/** The visible chat's context meter, drawn beside the IDE button. `percent` null hides it: the
 *  provider has not reported usage, which is not the same as 0%. */
// A type, not an interface, for invoke's args (see NativeSidebar).
export type NativeContext = {
  percent: number | null
  usedLabel: string
  canCompact: boolean
  compacting: boolean
}

/** The composer's model and effort picker, drawn as a native sheet. */
// A type, not an interface, for invoke's args (see NativeSidebar).
export type NativeModelPicker = {
  open: boolean
  providers: {
    id: string
    name: string
    /** A PNG data URL of the provider's icon; `symbol` (an SF Symbol) stands in without one. */
    icon: string | null
    symbol: string
    models: { id: string; name: string; detail: string | null }[]
  }[]
  selected: { providerId: string; modelId: string } | null
  /** The selected model's effort levels, in order; empty hides the effort control. */
  efforts: string[]
  effort: string | null
}

// Types, not interfaces, for invoke's args (see NativeSidebar).
type ConfirmOptions = {
  title: string
  message: string
  confirmLabel: string
  cancelLabel: string
  destructive: boolean
}
type PromptOptions = {
  title: string
  message: string | null
  value: string
  placeholder: string
  confirmLabel: string
}

export type NativeChromeIntent =
  | { type: 'start' }
  | { type: 'dismiss' }
  | { type: 'back' }
  | { type: 'confirmFingerprint' }
  | { type: 'submitAddress'; name: string; address: string }
  | { type: 'submitCode'; code: string }
  | { type: 'tap'; id: string }
  | { type: 'openChatList'; seq?: number }
  | { type: 'closeChatList'; seq?: number }
  | { type: 'openIde'; seq?: number }
  | { type: 'closeIde'; seq?: number }
  | { type: 'selectRow'; id: string }
  | { type: 'newThread' }
  | { type: 'selectProject'; id: string }
  | { type: 'selectHost'; id: string }
  | { type: 'toggleFold'; id: string }
  | { type: 'toggleProvider'; id: string; enabled: boolean }
  | { type: 'toggleProviderTools'; id: string; enabled: boolean }
  | { type: 'reorderProviders'; ids: string[] }
  | { type: 'setDefaultPermission'; level: PermissionLevel }
  | { type: 'openSettings' }
  | { type: 'closeSettings' }
  | { type: 'openFiles'; seq?: number }
  | { type: 'closeFiles'; seq?: number }
  | { type: 'toggleDir'; path: string }
  | { type: 'openFile'; path: string }
  | { type: 'selectTab'; id: string }
  | { type: 'closeTab'; id: string }
  | { type: 'newTab'; actionId: string }
  | { type: 'compact' }
  | { type: 'selectModel'; providerId: string; modelId: string }
  | { type: 'setEffort'; effort: string }
  | { type: 'closeModelPicker' }
  | { type: 'rowAction'; rowId: string; actionId: string }
  | { type: 'dragRow'; rowId: string | null }
  | { type: 'moveRow'; rowId: string; targetId: string; position: DropMode }
  | { type: 'openHostPicker' }
  | { type: 'closeHostPicker' }
  | { type: 'addHost' }
  | { type: 'revokeHost'; id: string }

/** Whether the phone's native chrome draws the controls, so the web must not draw its own. */
export const hasNativeChrome = () => IS_MOBILE && isTauri()

/** Replaces what the chrome shows; a part left out of `model` is hidden. */
export function setNativeChrome(model: Partial<NativeChrome>): void {
  if (!hasNativeChrome()) return
  void invoke(`plugin:${PLUGIN}|set_chrome`, model)
}

/** Replaces the native chat list's content. */
export function setNativeSidebar(sidebar: NativeSidebar): void {
  if (!hasNativeChrome()) return
  void invoke(`plugin:${PLUGIN}|set_sidebar`, sidebar)
}

/** Replaces the Agents settings page's content. */
export function setNativeAgentsSettings(settings: NativeAgentsSettings): void {
  if (!hasNativeChrome()) return
  void invoke(`plugin:${PLUGIN}|set_agents_settings`, settings)
}

/** Replaces the Hosts settings page's content. */
export function setNativeHostsSettings(settings: NativeHostsSettings): void {
  if (!hasNativeChrome()) return
  void invoke(`plugin:${PLUGIN}|set_hosts_settings`, settings)
}

/** Replaces the host picker's content. */
export function setNativeHostPicker(picker: NativeHostPicker): void {
  if (!hasNativeChrome()) return
  void invoke(`plugin:${PLUGIN}|set_host_picker`, picker)
}

/** Replaces the file drawer's content. */
export function setNativeFiles(files: NativeFiles): void {
  if (!hasNativeChrome()) return
  void invoke(`plugin:${PLUGIN}|set_files`, files)
}

/** Replaces the IDE's native tab strip. */
export function setNativeTabs(tabs: NativeTabs): void {
  if (!hasNativeChrome()) return
  void invoke(`plugin:${PLUGIN}|set_tabs`, tabs)
}

/** Replaces the context meter's content. */
export function setNativeContext(context: NativeContext): void {
  if (!hasNativeChrome()) return
  void invoke(`plugin:${PLUGIN}|set_context`, context)
}

/** Replaces the model picker's content. */
export function setNativeModelPicker(picker: NativeModelPicker): void {
  if (!hasNativeChrome()) return
  void invoke(`plugin:${PLUGIN}|set_model_picker`, picker)
}

/** Asks a native yes/no question over whatever is on screen; no without the phone's chrome. */
export async function nativeConfirm(options: ConfirmOptions): Promise<boolean> {
  if (!hasNativeChrome()) return false
  const answer = await invoke<{ confirmed: boolean }>(`plugin:${PLUGIN}|confirm`, options)
  return answer.confirmed
}

/** Asks for a line of text natively; null when cancelled or without the phone's chrome. */
export async function nativePrompt(options: PromptOptions): Promise<string | null> {
  if (!hasNativeChrome()) return null
  const answer = await invoke<{ value: string | null }>(`plugin:${PLUGIN}|prompt`, options)
  return answer.value
}

/** Subscribes to the chrome's intents; returns the unsubscribe. */
export function onNativeChromeIntent(cb: (intent: NativeChromeIntent) => void): () => void {
  if (!hasNativeChrome()) return () => {}
  let listener: PluginListener | null = null
  let stopped = false
  void addPluginListener(PLUGIN, 'intent', cb).then((registered) => {
    if (stopped) void registered.unregister()
    else listener = registered
  })
  return () => {
    stopped = true
    void listener?.unregister()
  }
}
