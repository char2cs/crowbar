import js from '@eslint/js'
import reactHooks from 'eslint-plugin-react-hooks'
import tseslint from 'typescript-eslint'
import globals from 'globals'

export default tseslint.config(
  // Build output — a standalone `ignores` entry is a global ignore in flat
  // config. Without it `eslint .` walks compiled bundles in dist/.
  { ignores: ['dist/**'] },
  {
    // Node build scripts run under Node, not the browser — give them the Node
    // globals (console/process/__dirname) so no-undef doesn't flag them.
    files: ['scripts/**/*.{js,mjs,cjs}'],
    languageOptions: { globals: { ...globals.node } },
  },
  js.configs.recommended,
  tseslint.configs.recommended,
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
      'no-restricted-imports': ['error', { patterns: ['@tauri-apps/*'] }],
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
)
