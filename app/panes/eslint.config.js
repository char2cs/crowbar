import js from '@eslint/js'
import eslintComments from '@eslint-community/eslint-plugin-eslint-comments/configs'
import vitest from '@vitest/eslint-plugin'
import reactHooks from 'eslint-plugin-react-hooks'
import tseslint from 'typescript-eslint'
import globals from 'globals'

const MAX_LINES_BASELINE = [
  'src/__tests__/components/sidebar/hooks/use-sidebar-drag.test.ts',
  'src/__tests__/components/sidebar/lib/drop-actions.test.ts',
  'src/__tests__/components/sidebar/lib/sidebar-drop-policy.test.ts',
  'src/__tests__/components/sidebar/sidebar-row.test.tsx',
  'src/__tests__/features/agent/composer/plate/chat-paste-plugin.test.tsx',
  'src/__tests__/features/agent/transcript/plate/streaming-value-patch.test.ts',
  'src/__tests__/features/panes/components/pane-container.test.tsx',
  'src/__tests__/features/settings/components/tabs/providers-settings.test.tsx',
]

const UNDESCRIBED_DIRECTIVE_BASELINE = [
  'src/features/tabs/hooks/use-pane-top-row-edges.ts',
  'src/features/terminal/hooks/use-terminal-connection.ts',
  'src/features/terminal/utils/input-tape.ts',
]
const NO_ASSERTION_BASELINE = ['src/__tests__/features/agent/composer/excalidraw-takeover.test.tsx']

export const BASELINES = [
  { rule: 'max-lines', files: MAX_LINES_BASELINE },
  {
    rule: '@eslint-community/eslint-comments/require-description',
    files: UNDESCRIBED_DIRECTIVE_BASELINE,
  },
  { rule: 'vitest/expect-expect', files: NO_ASSERTION_BASELINE },
]

const ENTITY = '(projects|chats|workspaces)'
const V0_TAIL = `\\x2fv0(\\x2f(${ENTITY}\\x2f?)?)?$`
const ENTITY_API_SELECTORS = [
  `Literal[value=/\\x2fv0\\x2f${ENTITY}\\x2f/]`,
  `TemplateElement[value.raw=/\\x2fv0\\x2f${ENTITY}\\x2f/]`,
  `BinaryExpression[operator='+'] > Literal[value=/${V0_TAIL}/]`,
  `BinaryExpression[operator='+'] > TemplateLiteral > TemplateElement[value.raw=/${V0_TAIL}/]`,
  `TemplateElement[tail=false][value.raw=/${V0_TAIL}/]`,
  `TemplateElement[value.raw=/^${ENTITY}\\x2f/]`,
  'Literal[value=/^\\x2f?v0\\x2f?$/]',
]
const ENTITY_API_PATH_MESSAGE =
  'Build project/repo/workspace/chat URLs with projectBase/repoBase (lib/project-url) or the ' +
  'lib/workspace-scope-url builders: a hand-written path carries no host and lands on the local daemon.'

const PROVIDER_NAME = '(claude|codex|anthropic|openai)'
const PROVIDER_VOCABULARY_RULES = [
  {
    selector: `Literal[value=/${PROVIDER_NAME}/i]`,
    message:
      'Crowbar is provider-agnostic: read the provider catalog (label, icon, capabilities), not a hard-coded name.',
  },
  {
    selector: `TemplateElement[value.raw=/${PROVIDER_NAME}/i]`,
    message:
      'Crowbar is provider-agnostic: read the provider catalog (label, icon, capabilities), not a hard-coded name.',
  },
]

const WINDOW_WIDTH_RULES = [
  {
    selector:
      "CallExpression:matches([callee.name='matchMedia'], [callee.property.name='matchMedia']) > :matches(Literal[value=/width/], TemplateLiteral).arguments",
    message:
      'A width media query asks the pane, not the window: use the sm:/md:/lg:/xl: variants, which follow the shell window.',
  },
]

const SWALLOWED_FAILURE_RULES = [
  {
    selector: 'CatchClause > BlockStatement[body.length=0]',
    message: 'An empty catch hides a failure: return the fallback, or report() it.',
  },
  {
    selector:
      "CallExpression[callee.property.name='catch'] > ArrowFunctionExpression.arguments[body.type='BlockStatement'][body.body.length=0]",
    message: '.catch(() => {}) hides a failure: report() it.',
  },
  {
    selector:
      "CallExpression[callee.property.name='catch'] > FunctionExpression.arguments[body.body.length=0]",
    message: '.catch(function () {}) hides a failure: report() it.',
  },
  {
    selector:
      "CallExpression[callee.property.name='then'][arguments.length=2] > :matches(ArrowFunctionExpression, FunctionExpression).arguments:last-child[body.type='BlockStatement'][body.body.length=0]",
    message: '.then(onDone, () => {}) hides a failure: report() it.',
  },
  {
    selector:
      "CallExpression[callee.property.name='catch'] > ArrowFunctionExpression.arguments[body.type='Literal']",
    message: '.catch(() => null/false/…) hides a failure: handle it or report() it.',
  },
  {
    selector:
      "CallExpression[callee.property.name='catch'] > ArrowFunctionExpression.arguments[body.operator='void'][body.argument.type='Literal']",
    message: '.catch(() => void 0) hides a failure: handle it or report() it.',
  },
  {
    selector:
      "CallExpression[callee.property.name='catch'] > ArrowFunctionExpression.arguments[body.type='Identifier'][body.name='undefined']",
    message: '.catch(() => undefined) hides a failure: handle it or report() it.',
  },
  {
    selector:
      "CallExpression[callee.object.name=/^(toast|toastManager)$/] :matches(ConditionalExpression[test.operator='instanceof'][test.right.name='Error'], MemberExpression[object.name=/^(e|err|error)$/][property.name='message'], CallExpression[callee.name='String'][arguments.0.name=/^(e|err|error)$/])",
    message: "A toast of a thrown value's text skips report(): report(err, { action }) it.",
  },
]

const TAURI_IMPORTS = {
  group: ['@tauri-apps/*'],
  message: 'Import Tauri APIs through the bridge modules in src/lib/.',
}

export default tseslint.config(
  { ignores: ['dist/**', 'dist-panes/**', 'public/mockServiceWorker.js', 'src/routeTree.gen.ts'] },
  {
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
    files: ['src/**/*.{ts,tsx}'],
    ignores: ['src/__tests__/**'],
    rules: {
      'no-restricted-syntax': [
        'error',
        ...SWALLOWED_FAILURE_RULES,
        ...WINDOW_WIDTH_RULES,
        ...ENTITY_API_SELECTORS.map((selector) => ({ selector, message: ENTITY_API_PATH_MESSAGE })),
      ],
    },
  },
  {
    files: ['src/features/agent/**/*.{ts,tsx}', 'src/lib/**/*.{ts,tsx}'],
    ignores: ['src/__tests__/**'],
    rules: {
      'no-restricted-syntax': [
        'error',
        ...SWALLOWED_FAILURE_RULES,
        ...WINDOW_WIDTH_RULES,
        ...ENTITY_API_SELECTORS.map((selector) => ({ selector, message: ENTITY_API_PATH_MESSAGE })),
        ...PROVIDER_VOCABULARY_RULES,
      ],
    },
  },
  {
    files: ['src/lib/host-path.ts', 'src/lib/project-url.ts', 'src/lib/workspace-scope-url.ts'],
    rules: {
      'no-restricted-syntax': ['error', ...SWALLOWED_FAILURE_RULES, ...WINDOW_WIDTH_RULES],
    },
  },
  {
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
    files: [
      'src/lib/crowbar-bridge.ts',
      'src/lib/ws/tauri-transport.ts',
      'src/lib/native-dialog.ts',
      'src/lib/external-open.ts',
      'src/lib/browser-bridge.ts',
      'src/lib/native-chrome.ts',
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
  ...BASELINES.filter(({ files }) => files.length > 0).map(({ rule, files }) => ({
    name: `baseline/${rule}`,
    files,
    rules: { [rule]: 'off' },
  })),
)
