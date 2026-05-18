import js from '@eslint/js'

export default [
  js.configs.recommended,
  {
    files: ['**/*.{ts,tsx}'],
    rules: {
      'no-restricted-imports': ['error', { patterns: ['@tauri-apps/*'] }],
    },
  },
]
