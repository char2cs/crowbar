import js from '@eslint/js'
import eslintComments from '@eslint-community/eslint-plugin-eslint-comments/configs'
import vitest from '@vitest/eslint-plugin'
import reactHooks from 'eslint-plugin-react-hooks'
import tseslint from 'typescript-eslint'
import globals from 'globals'

// Files over the 600-line cap when the cap landed (2026-09-24). This list is
// the backlog: split a file, then delete its entry. Never add to it.
const MAX_LINES_BASELINE = [
  'src/__tests__/components/app-sync-provider-folders.test.tsx',
  'src/__tests__/components/layout/sidebar-carousel.test.tsx',
  'src/__tests__/components/sidebar/hooks/use-sidebar-drag.test.ts',
  'src/__tests__/components/sidebar/lib/drop-actions.test.ts',
  'src/__tests__/components/sidebar/lib/row-actions.test.ts',
  'src/__tests__/components/sidebar/lib/rows-from-repo.test.ts',
  'src/__tests__/components/sidebar/lib/sidebar-drop-policy.test.ts',
  'src/__tests__/components/sidebar/sidebar-row.test.tsx',
  'src/__tests__/components/sidebar/space-header.test.tsx',
  'src/__tests__/components/sidebar/space-scroller.test.tsx',
  'src/__tests__/features/agent/api/agent-api.test.ts',
  'src/__tests__/features/agent/chat/agent-chat-view.test.tsx',
  'src/__tests__/features/agent/chat/agent-empty-document.test.tsx',
  'src/__tests__/features/agent/components/agent-chat-pane.test.tsx',
  'src/__tests__/features/agent/composer/agent-composer.test.tsx',
  'src/__tests__/features/agent/composer/composer-choice.test.tsx',
  'src/__tests__/features/agent/composer/plate/chat-markdown-editor.test.tsx',
  'src/__tests__/features/agent/composer/plate/chat-paste-plugin.test.tsx',
  'src/__tests__/features/agent/hooks/use-chat-messages.test.ts',
  'src/__tests__/features/agent/hooks/use-transcript-anchor.test.tsx',
  'src/__tests__/features/agent/transcript/agent-transcript.test.tsx',
  'src/__tests__/features/agent/transcript/plate/streaming-value-patch.test.ts',
  'src/__tests__/features/agent/tree/lib/chat-rows.test.ts',
  'src/__tests__/features/git/components/diff/use-review-annotations.test.tsx',
  'src/__tests__/features/panes/components/pane-container.test.tsx',
  'src/__tests__/features/panes/hooks/use-pane-keyboard.test.ts',
  'src/__tests__/features/settings/components/tabs/providers-settings.test.tsx',
  'src/__tests__/features/terminal/hooks/use-terminal-connection.test.ts',
  'src/__tests__/features/workspace/stores/hooks/use-workspace-agent-chats-stream.test.ts',
  'src/__tests__/features/workspace/stores/hooks/use-workspace-effects.test.ts',
  'src/__tests__/features/workspace/stores/slices/agent-chats-slice.test.ts',
  'src/__tests__/lib/persistence/hydrate.test.ts',
  'src/components/app-sync-engine.ts',
  'src/components/sidebar/hooks/use-sidebar-drag.ts',
  'src/components/sidebar/lib/drop-actions.ts',
  'src/components/sidebar/lib/row-actions.ts',
  'src/components/sidebar/lib/rows-from-repo.ts',
  'src/components/sidebar/sidebar-row.tsx',
  'src/components/ui/dropdown.tsx',
  'src/components/ui/table-icons.tsx',
  'src/components/ui/table-node.tsx',
  'src/features/agent/api/agent-api.ts',
  'src/features/agent/chat/agent-chat-view.tsx',
  'src/features/agent/components/agent-chat-pane.tsx',
  'src/features/agent/hooks/use-prompt-queue.ts',
  'src/features/agent/hooks/use-transcript-anchor.ts',
  'src/features/agent/transcript/agent-transcript.tsx',
  'src/features/agent/transcript/plate/streaming-value-patch.ts',
  'src/features/file-explorer/file-explorer/hooks/use-file-explorer-context-menu.tsx',
  'src/features/tabs/components/tab-bar.tsx',
  'src/features/workspace/stores/slices/agent-chats-slice.ts',
  'src/lib/api.ts',
]

// Findings that predate these rules, in areas being rewritten by the
// stabilization plan (docs/plans/2026-09-24-stabilization.md §7). Each entry
// goes when its file is fixed; never add to these lists.
const EXHAUSTIVE_DEPS_BASELINE = ['src/features/agent/transcript/agent-transcript.tsx']
const UNDESCRIBED_DIRECTIVE_BASELINE = [
  'src/__tests__/features/terminal/hooks/use-terminal-connection.test.ts',
  'src/features/agent/transcript/plate/chat-fresh-text-plugin.tsx',
  'src/features/editor/hooks/use-pane-editor-controller.ts',
  'src/features/panes/components/pane-sash.tsx',
  'src/features/tabs/hooks/use-pane-top-row-edges.ts',
  'src/features/terminal/hooks/use-terminal-connection.ts',
  'src/features/terminal/utils/input-tape.ts',
  'src/features/workspace/stores/hooks/use-workspace-effects.ts',
]
const NO_ASSERTION_BASELINE = [
  'src/__tests__/features/agent/composer/agent-composer.test.tsx',
  'src/__tests__/features/agent/composer/excalidraw-takeover.test.tsx',
]

export const BASELINES = [
  { rule: 'max-lines', files: MAX_LINES_BASELINE },
  { rule: 'react-hooks/exhaustive-deps', files: EXHAUSTIVE_DEPS_BASELINE },
  {
    rule: '@eslint-community/eslint-comments/require-description',
    files: UNDESCRIBED_DIRECTIVE_BASELINE,
  },
  { rule: 'vitest/expect-expect', files: NO_ASSERTION_BASELINE },
]

const TAURI_IMPORTS = {
  group: ['@tauri-apps/*'],
  message: 'Import Tauri APIs through the bridge modules in src/lib/.',
}

export default tseslint.config(
  // Build output — a standalone `ignores` entry is a global ignore in flat
  // config. Without it `eslint .` walks compiled bundles in dist/.
  // Generated files are ignored too: their headers carry blanket disables.
  { ignores: ['dist/**', 'public/mockServiceWorker.js', 'src/routeTree.gen.ts'] },
  {
    // Node build scripts run under Node, not the browser — give them the Node
    // globals (console/process/__dirname) so no-undef doesn't flag them.
    files: ['scripts/**/*.{js,mjs,cjs}'],
    languageOptions: { globals: { ...globals.node } },
  },
  js.configs.recommended,
  tseslint.configs.recommended,
  eslintComments.recommended,
  {
    linterOptions: { reportUnusedDisableDirectives: 'error' },
    rules: {
      '@eslint-community/eslint-comments/require-description': 'error',
    },
  },
  {
    // Classic react-hooks rules only (v7's recommended preset would also
    // enable the React Compiler rules — not adopted yet).
    plugins: { 'react-hooks': reactHooks },
    rules: {
      'react-hooks/rules-of-hooks': 'error',
      'react-hooks/exhaustive-deps': 'warn',
    },
  },
  {
    files: ['**/*.{ts,tsx}'],
    rules: {
      'max-lines': ['error', { max: 600 }],
      'no-restricted-imports': ['error', { patterns: [TAURI_IMPORTS] }],
      '@typescript-eslint/no-explicit-any': 'error',
      '@typescript-eslint/ban-ts-comment': [
        'error',
        {
          'ts-expect-error': 'allow-with-description',
          'ts-ignore': true,
          'ts-nocheck': true,
          minimumDescriptionLength: 10,
        },
      ],
      '@typescript-eslint/no-unused-vars': [
        'error',
        {
          argsIgnorePattern: '^_',
          varsIgnorePattern: '^_',
          caughtErrors: 'none',
        },
      ],
      '@typescript-eslint/no-unused-expressions': [
        'error',
        { allowShortCircuit: true, allowTernary: true },
      ],
    },
  },
  {
    // Stores hold state; side effects that need UI live in components that
    // watch the store (CLAUDE.md "Store patterns").
    files: ['src/**/stores/**/*.{ts,tsx}'],
    rules: {
      'no-restricted-imports': [
        'error',
        {
          patterns: [
            TAURI_IMPORTS,
            {
              group: ['@/components/*', '@/features/*/components/*', '**/components/*'],
              message:
                'Stores must not import components; watch store state from a component instead.',
            },
          ],
        },
      ],
    },
  },
  {
    // The vetted bridge modules — the only files allowed to import
    // `@tauri-apps/*` directly. Everything else goes through them.
    files: [
      'src/lib/crowbar-bridge.ts',
      'src/lib/ws/tauri-transport.ts',
      'src/lib/native-dialog.ts',
      'src/lib/external-open.ts',
    ],
    rules: {
      'no-restricted-imports': 'off',
    },
  },
  {
    files: ['src/__tests__/**/*.{ts,tsx}'],
    plugins: { vitest },
    rules: {
      'vitest/no-focused-tests': 'error',
      'vitest/no-disabled-tests': 'error',
      // Assertion helpers (expect*/assert*), awaited findBy* queries and
      // when* waiters (vi.waitFor around an expect) count as assertions.
      'vitest/expect-expect': [
        'error',
        {
          assertFunctionNames: [
            'expect',
            'expect*',
            'assert*',
            '*.expect*',
            '*.findBy*',
            'findBy*',
            'when*',
            'runSequence',
          ],
        },
      ],
    },
  },
  // Named `baseline/*` so scripts/check-eslint-baselines.mjs can drop them and
  // fail on any listed file that no longer violates (baselines only shrink).
  ...BASELINES.filter(({ files }) => files.length > 0).map(({ rule, files }) => ({
    name: `baseline/${rule}`,
    files,
    rules: { [rule]: 'off' },
  })),
)
