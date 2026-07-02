import js from '@eslint/js'
import reactHooks from 'eslint-plugin-react-hooks'
import tseslint from 'typescript-eslint'

export default tseslint.config(
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
