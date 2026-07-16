import type { ReactDoctorConfig } from 'react-doctor/api'

// React Doctor configuration for Crowbar Web.
//
// The rule exceptions below were adjudicated during the batch-1 error review
// (2026-07-14) by reading every flagged site and independently sample-verifying
// the verdicts. Full evidence: .superpowers/sdd/task-19-report.md.
//
// `rules` values: "off" skips the rule entirely (never runs, drops out of the
// score); "warn" / "error" restamp severity. Keys are fully-qualified
// `<plugin>/<rule>`. Anything not listed keeps its built-in severity.
export default {
  rules: {
    // OFF — false-positive-dominated (42/42 sampled). Every flagged site is one
    // of this codebase's sanctioned render-time ref patterns: latest-ref
    // (`ref.current = x` to keep a stable closure reading the freshest value),
    // null-guarded lazy-init, and deepEqual/adjust-during-render. These were
    // deliberately adopted by the performance program; the rule (whose own help
    // text says the lazy-init pattern "remains supported") is incompatible with
    // them and found zero real defects.
    'react-doctor/no-ref-current-in-render': 'off',

    // OFF — false-positive-dominated (13/13 sampled). The rule flags any function
    // that calls a state setter as an "impure functional updater", mislabeling
    // ordinary event handlers and stable-callback wrappers. The codebase's genuine
    // functional updaters (`setX(prev => …)`) are pure and go unflagged; no real
    // impure updater was found.
    'react-doctor/no-impure-state-updater': 'off',

    // OFF — not applicable. This repo is bun-only; pnpm was retired in PR #46, so
    // there is no pnpm install path to harden (the rule's `minimumReleaseAge` /
    // `trustPolicy` knobs live in a pnpm-workspace.yaml this repo does not install
    // from).
    'react-doctor/require-pnpm-hardening': 'off',

    // NOTE — `effect-needs-cleanup` is intentionally left ACTIVE: it caught 5 real
    // subscription/timer leaks in batch 1. Its 9 residual false positives (cleanup
    // exists via indirection the static tracer cannot follow) are suppressed
    // per-site with `// react-doctor-disable-next-line effect-needs-cleanup`
    // comments rather than by disabling the rule here.

    // OFF — false-positive-dominated (batch-3 review, 2026-07-14; re-verified in
    // batch 6 on a fresh sample — monaco-diff-editor:791/796/822 and
    // use-file-explorer-sync:43 all still FP). Every flagged site is an
    // imperative-handle or controlled-value coordination pattern deliberately used
    // in this codebase (monaco-diff-editor / terminal.tsx registering a resolver
    // handle up to a parent, font-selector's once-loaded autocorrect of a
    // controlled value, use-file-explorer-sync whose entire declared purpose is
    // editor→explorer path sync) rather than an accidental parent-sync side effect.
    // Full per-site trace: .superpowers/sdd/task-21-report.md. NOTE: react-doctor
    // 0.7.7 renamed these rules from parent-sync-via-callback-effect /
    // data-passed-to-parent-via-effect to the slugs below; the old keys were
    // no-ops (the rules kept firing) until this correction.
    'react-doctor/no-prop-callback-in-effect': 'off',
    'react-doctor/no-pass-data-to-parent': 'off',

    // OFF — false-positive-dominated (8/8 sampled, 2026-07-15) and categorically
    // unwinnable in this codebase. The rule (Fast-Refresh "only export
    // components") is a DEV-only HMR-ergonomics check with zero production impact;
    // it fires on sanctioned co-location patterns, never a real defect:
    //   • routes/** — every TanStack Router file MUST `export const Route`
    //     alongside its inline route component (createFileRoute). This is
    //     framework-mandated and fires across the whole route tree, so no refactor
    //     can reach a clean score while the rule is active (home.tsx:55 sampled).
    //   • Context modules co-locate their consumer hooks with the Provider — the
    //     canonical React pattern; splitting them would fragment a cohesive unit
    //     for no runtime benefit (workspace-tree-context useWorkspaceTreeActions/
    //     useWorkspaceTreeDrag, ui/sidebar useSidebar, ui/context-menu
    //     useContextMenu).
    //   • Primitive passthroughs / module singletons live with their component by
    //     design (ui/command CommandCreateHandle re-export, ui/toast toastManager/
    //     anchoredToastManager). All are stable, rarely-edited files where HMR
    //     state preservation is moot.
    'react-doctor/only-export-components': 'off',
  },
} satisfies ReactDoctorConfig
