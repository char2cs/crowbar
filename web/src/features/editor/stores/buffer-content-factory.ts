import { detectLanguageFromFileName } from '@/features/editor/utils/language-detection'
import type { OpenContentSpec, PaneContent } from '@/features/panes/types/pane-content'

export const createPaneContent = (id: string, spec: OpenContentSpec): PaneContent => {
  const base = {
    id,
    isPinned: false,
    isActive: true,
  }

  switch (spec.type) {
    case 'editor':
      return {
        ...base,
        type: 'editor',
        path: spec.path,
        name: spec.name,
        content: spec.content,
        savedContent: spec.content,
        isDirty: false,
        isVirtual: spec.isVirtual ?? false,
        isPreview: spec.isPreview ?? false,
        language: spec.language ?? detectLanguageFromFileName(spec.name),
        tokens: [],
      }
    case 'terminal': {
      const sessionId = spec.sessionId ?? id.replace('buffer_', '')
      return {
        ...base,
        type: 'terminal',
        path: spec.path ?? `terminal://${sessionId}`,
        name: spec.name ?? 'Terminal',
        isPreview: false,
        sessionId,
        initialCommand: spec.command,
        workingDirectory: spec.workingDirectory,
        remoteConnectionId: spec.remoteConnectionId,
      }
    }
    case 'webViewer':
      return {
        ...base,
        type: 'webViewer',
        path: `web-viewer://${spec.url}`,
        name: 'Web Viewer',
        isPreview: false,
        url: spec.url,
        zoomLevel: spec.zoomLevel,
        profileKey: spec.profileKey,
        history: spec.history,
        historyIndex: spec.historyIndex,
      }
    case 'newTab':
      return {
        ...base,
        type: 'newTab',
        path: `newtab://${id}`,
        name: 'New Tab',
        isPreview: false,
      }
    case 'diff':
      return {
        ...base,
        type: 'diff',
        path: spec.path,
        name: spec.name,
        isPreview: false,
        content: spec.content,
        savedContent: spec.content,
        diffData: spec.diffData,
      }
    case 'markdownPreview':
      return {
        ...base,
        type: 'markdownPreview',
        path: spec.path,
        name: spec.name,
        isPreview: false,
        content: spec.content,
        sourceFilePath: spec.sourceFilePath,
      }
    case 'htmlPreview':
      return {
        ...base,
        type: 'htmlPreview',
        path: spec.path,
        name: spec.name,
        isPreview: false,
        content: spec.content,
        sourceFilePath: spec.sourceFilePath,
      }
    case 'csvPreview':
      return {
        ...base,
        type: 'csvPreview',
        path: spec.path,
        name: spec.name,
        isPreview: false,
        content: spec.content,
        sourceFilePath: spec.sourceFilePath,
      }
    case 'externalEditor':
      return {
        ...base,
        type: 'externalEditor',
        path: spec.path,
        name: spec.name,
        isPreview: false,
        terminalConnectionId: spec.terminalConnectionId,
      }
    case 'crowbarChat':
      return {
        ...base,
        type: 'crowbarChat',
        path: `crowbar-chat://${spec.wsId}`,
        name: spec.name,
        isPreview: false,
        wsId: spec.wsId,
      }
  }
}
