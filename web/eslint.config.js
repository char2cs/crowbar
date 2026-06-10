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
    },
  },
)
