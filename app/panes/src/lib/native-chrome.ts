import { addPluginListener, invoke, type PluginListener } from '@tauri-apps/api/core'
import { isTauri } from '@/lib/crowbar-bridge'
import { IS_MOBILE } from '@/utils/platform'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import type { AllowedModes, DropMode } from '@/components/tree-dnd/drop-core'
import type { HostDotState } from '@/features/hosts/types'
import type { HostNetState, NetState, ReadoutLines } from '@/components/layout/build-readout-lines'
import type { Project } from '@/lib/types'
import type { ThemeMode } from '@/features/settings/types/settings'
import type { AgentProvider, PermissionLevel } from '@/features/agent/api/agent-providers-api'
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

export interface ChromeButton {
  id: string
  icon: string
  side: 'leading' | 'trailing'
  label: string
  enabled: boolean
  intent?: string
}

export interface ChromeHeaders {
  chat: ChromeButton[]
  ide: ChromeButton[]
}

export type ChromeReveal = 'none' | 'chatList' | 'ide' | 'files'

export interface NativeChrome {
  accent: string
  welcome: { visible: boolean; label: string }
  brandGround: string
  connect: ConnectSheet
  headers: ChromeHeaders
  reveal: ChromeReveal
  ack: number
  hostPlace: 'local' | 'remote'
  hostStatus: HostDotState
  appearance: 'light' | 'dark' | 'system'
  settings: {
    open: boolean
    theme: string
    themes: { id: string; name: string }[]
    pages: { id: string; label: string; icon: string }[]
  }
  palette: NativePalette
}

export interface NativePalette {
  surface: string
  raised: string
  text: string
  secondaryText: string
  brandLight: string
  brandDark: string
  sidebar: {
    text: string
    brand: string
    brandText: string
  } | null
}

export type NativeSidebar = {
  focusedChatId: string | null
  activeProjectId: string | null
  projects: Project[]
  hosts: { id: string; name: string; state: HostDotState }[]
  rows: SidebarRow[]
  active: { id: string }[]
  collapsed: string[]
  actions: Record<string, NativeRowAction[]>
  drop: NativeRowDrop | null
  readout: NativeReadout | null
}

export type NativeReadout = {
  lines: ReadoutLines
  net: NetState
  hosts: { name: string; net: HostNetState }[]
}

export type NativeRowDrop = {
  subjectId: string
  targets: Record<string, { modes: AllowedModes; band: number }>
}

export type NativeRowAction = {
  id: string
  title: string
  icon: string
  destructive: boolean
  confirm: { title: string; message: string } | null
}

export type NativeHostsSettings = {
  hosts: { id: string; name: string; endpoint: string; state: HostDotState }[]
  revoking: string[]
  shownHost: string | null
}

export type NativeHostPicker = NativeHostsSettings & { open: boolean }

export type NativeAgentsSettings = {
  hostId: string
  hostName: string
  status: AgentProvidersStatus
  providers: AgentProvider[]
  permissionLevel: PermissionLevel
}

export type NativeFilesEntry = {
  name: string
  path: string
  isDir: boolean
  isSymlink?: boolean
  ignored?: boolean
  git: FileTreeGitStatusDecoration | null
  children?: NativeFilesEntry[]
}

export type NativeFiles = {
  header: ChatHeaderModel | null
  loading: boolean
  files: NativeFilesEntry[]
  expanded: string[]
  activePath: string | null
  compactFolders: boolean
}

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
  newTabActions: { id: string; title: string; icon: string }[]
}

export type NativeModelPicker = {
  open: boolean
  providers: {
    id: string
    name: string
    icon: string | null
    fallbackIcon: string
    models: { id: string; name: string }[]
  }[]
  selected: { providerId: string; modelId: string } | null
  efforts: string[]
  effort: string | null
  permission: {
    levels: { value: PermissionLevel; label: string; description: string }[]
    level: PermissionLevel | ''
  } | null
}

export type NativePlusMenu = {
  open: boolean
  title: string
  items: {
    id: string
    title: string
    icon: string
    hint: string | null
    divider: boolean
  }[]
}

export type NativeAttachSheet = {
  open: boolean
  title: string
  note: string
  sources: { id: string; title: string; icon: string }[]
  files: {
    id: string
    name: string
    detail: string
    status: 'uploading' | 'done' | 'bad'
    percent: number | null
    canRetry: boolean
  }[]
  hint: string | null
  attachLabel: string
  canAttach: boolean
}

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
  | { type: 'setThemeMode'; mode: ThemeMode }
  | { type: 'setTheme'; id: string }
  | { type: 'openSettings' }
  | { type: 'closeSettings' }
  | { type: 'openFiles'; seq?: number }
  | { type: 'closeFiles'; seq?: number }
  | { type: 'toggleDir'; path: string }
  | { type: 'openFile'; path: string }
  | { type: 'selectTab'; id: string }
  | { type: 'closeTab'; id: string }
  | { type: 'newTab'; actionId: string }
  | { type: 'selectModel'; providerId: string; modelId: string }
  | { type: 'setEffort'; effort: string }
  | { type: 'setPermissionLevel'; level: PermissionLevel }
  | { type: 'closeModelPicker' }
  | { type: 'pickPlusItem'; id: string }
  | { type: 'closePlusMenu' }
  | { type: 'pickAttachSource'; id: string }
  | { type: 'removeAttachFile'; id: string }
  | { type: 'retryAttachFile'; id: string }
  | { type: 'attachFiles' }
  | { type: 'closeAttachSheet' }
  | { type: 'jumpToLatest'; owner: string }
  | { type: 'rowAction'; rowId: string; actionId: string }
  | { type: 'dragRow'; rowId: string | null }
  | { type: 'moveRow'; rowId: string; targetId: string; position: DropMode }
  | { type: 'openHostPicker' }
  | { type: 'closeHostPicker' }
  | { type: 'addHost' }
  | { type: 'revokeHost'; id: string }

export const hasNativeChrome = () => IS_MOBILE && isTauri()

export function setNativeChrome(model: Partial<NativeChrome>): void {
  if (!hasNativeChrome()) return
  void invoke(`plugin:${PLUGIN}|set_chrome`, model)
}

export function setNativeSidebar(sidebar: NativeSidebar): void {
  if (!hasNativeChrome()) return
  void invoke(`plugin:${PLUGIN}|set_sidebar`, sidebar)
}

export function setNativeAgentsSettings(settings: NativeAgentsSettings): void {
  if (!hasNativeChrome()) return
  void invoke(`plugin:${PLUGIN}|set_agents_settings`, settings)
}

export function setNativeHostsSettings(settings: NativeHostsSettings): void {
  if (!hasNativeChrome()) return
  void invoke(`plugin:${PLUGIN}|set_hosts_settings`, settings)
}

export function setNativeHostPicker(picker: NativeHostPicker): void {
  if (!hasNativeChrome()) return
  void invoke(`plugin:${PLUGIN}|set_host_picker`, picker)
}

export function setNativeFiles(files: NativeFiles): void {
  if (!hasNativeChrome()) return
  void invoke(`plugin:${PLUGIN}|set_files`, files)
}

export function setNativeTabs(tabs: NativeTabs): void {
  if (!hasNativeChrome()) return
  void invoke(`plugin:${PLUGIN}|set_tabs`, tabs)
}

export function setNativeModelPicker(picker: NativeModelPicker): void {
  if (!hasNativeChrome()) return
  void invoke(`plugin:${PLUGIN}|set_model_picker`, picker)
}

export function setNativePlusMenu(menu: NativePlusMenu): void {
  if (!hasNativeChrome()) return
  void invoke(`plugin:${PLUGIN}|set_plus_menu`, menu)
}

export function setNativeAttachSheet(sheet: NativeAttachSheet): void {
  if (!hasNativeChrome()) return
  void invoke(`plugin:${PLUGIN}|set_attach_sheet`, sheet)
}

export type NativeJumpToLatest = { owner: string; visible: boolean; bottom: number; label: string }

export function setNativeJumpToLatest(state: NativeJumpToLatest): void {
  if (!hasNativeChrome()) return
  void invoke(`plugin:${PLUGIN}|set_jump_to_latest`, state)
}

export async function nativeConfirm(options: ConfirmOptions): Promise<boolean> {
  if (!hasNativeChrome()) return false
  const answer = await invoke<{ confirmed: boolean }>(`plugin:${PLUGIN}|confirm`, options)
  return answer.confirmed
}

export async function nativePrompt(options: PromptOptions): Promise<string | null> {
  if (!hasNativeChrome()) return null
  const answer = await invoke<{ value: string | null }>(`plugin:${PLUGIN}|prompt`, options)
  return answer.value
}

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
