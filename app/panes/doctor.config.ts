import type { ReactDoctorConfig } from 'react-doctor/api'

export default {
  ignore: {
    files: ['dist-panes/**'],
    overrides: [
      {
        files: [
          'src/entries/chat.tsx',
          'src/entries/console.tsx',
          'src/entries/editor.tsx',
          'src/entries/review.tsx',
        ],
        rules: ['deslop/unused-file'],
      },
    ],
  },
  rules: {
    'react-doctor/no-ref-current-in-render': 'off',

    'react-doctor/no-impure-state-updater': 'off',

    // per-site with `// react-doctor-disable-next-line effect-needs-cleanup`

    // Full per-site trace: .superpowers/sdd/task-21-report.md. NOTE: react-doctor
    'react-doctor/no-prop-callback-in-effect': 'off',
    'react-doctor/no-pass-data-to-parent': 'off',

    'react-doctor/only-export-components': 'off',
  },
} satisfies ReactDoctorConfig
