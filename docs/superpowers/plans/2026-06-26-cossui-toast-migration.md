# CossUI Toast Migration Plan

## Goal
Replace Sonner with CossUI Toast (`@base-ui/react`-backed), strip the dead notifications history system entirely, and keep the existing `toast.*` call-site API unchanged.

## Global Constraints
- Never hardcode colors — always use CSS variable tokens
- Always use `@/components/ui/*` — no raw `@base-ui/react` usage at call sites
- Component files use kebab-case filenames
- Test files live in `web/src/__tests__/` mirroring `web/src/`
- `toast.info/success/warning/error/show/dismiss/dismissByKey` public API must remain identical — zero changes at existing call sites
- TypeScript must compile clean (`pnpm --filter web tsc --noEmit`)
- No Zustand in the new toast system — CossUI manages its own state
- `sonner` package must be removed from `web/package.json` after migration

## Context

### Current architecture
- **`components/ui/sonner.tsx`** — mounts `<Sonner>` at root, reads `data-theme-type`
- **`components/ui/toast.tsx`** — exports `useToast`, `ToastContainer`, re-exports `toast` + `useToastStore`
- **`features/window/stores/toast-store.ts`** — Zustand store wrapping `sonner`'s imperative API; also maintains a `notifications: NotificationEntry[]` history array (unused by any real UI — the sidebar is a stub returning null)
- **`features/layout/contexts/toast-context.tsx`** — thin passthrough context; only used by `git-branch-manager.tsx`
- **`routes/__root.tsx`** — mounts `<Toaster />` from `components/ui/sonner`
- **`features/window/components/notifications-sidebar.tsx`** — stub returning null

### CossUI Toast API
```ts
// Provider at root
<ToastProvider position="bottom-right">
  <AnchoredToastProvider>
    ...
  </AnchoredToastProvider>
</ToastProvider>

// Imperative API
toastManager.add({ title, description?, type?, id?, timeout?, actionProps? })
toastManager.close(id)
toastManager.promise(promise, { loading, success, error })

// actionProps shape
actionProps: { children: ReactNode, onClick: () => void }
```

### Field mapping (current → CossUI)
| Current `toast.show()` field | CossUI `toastManager.add()` field |
|---|---|
| `message` | `title` |
| `description` | `description` |
| `type` | `type` |
| `key` | `id` (dedup by stable id) |
| `duration` | `timeout` |
| `action.label` | `actionProps.children` |
| `action.onClick` | `actionProps.onClick` |

### Files that call `toast.*` (no changes needed if adapter preserves API)
- `features/editor/components/toolbar/editor-status-actions.tsx` — `toast.show({ key, type, message, duration })`, `toast.dismissByKey(key)`, `toast.error(...)`
- `features/workspace/lib/external-buffer-sync.ts` — `toast.error(...)`
- `features/workspace/lib/open-file-content.ts` — `toast.error(...)`
- `features/editor/stores/editor-app-store.ts` — `toast.error(...)`
- `features/git/api/git-status-api.ts` — `toast.error(...)`
- `features/git/components/commit-popover.tsx` — `toast.error(...)`
- `features/git/components/review-thread-item.tsx` — `toast.error(...)`
- `features/settings/components/tabs/developer-settings.tsx` — `toast.success(...)`
- `components/projects/add-repository-modal.tsx` — `toast.error/success(...)`
- `components/projects/import-project-modal.tsx` — `toast.error/success(...)`
- `components/layout/repo-settings-panel.tsx` — `toast.error/success(...)`
- `components/workspace/new-workspace-page.tsx` — `toast.error(...)`

---

## Task 1: Install CossUI Toast + wire up provider + rewrite toast adapter

**Scope:** `web/` directory.

### Steps

1. **Install CossUI toast component** via the configured registry:
   ```bash
   cd web && pnpm dlx shadcn@latest add @coss/toast --overwrite
   ```
   This writes `web/src/components/ui/toast.tsx` with CossUI's component code. Accept all prompts / pass `--yes` if available.

2. **Rewrite `features/window/stores/toast-store.ts`** — replace the entire file. No Zustand. No notifications. Just a thin `toast` object that wraps `toastManager`:

   ```ts
   import { toastManager } from '@/components/ui/toast'

   interface ShowOptions {
     message: string
     description?: string
     type?: 'info' | 'success' | 'warning' | 'error'
     key?: string
     duration?: number
     action?: { label: string; onClick: () => void }
   }

   function toAdd(opts: ShowOptions) {
     return {
       title: opts.message,
       description: opts.description,
       type: opts.type,
       id: opts.key,
       timeout: opts.duration,
       actionProps: opts.action
         ? { children: opts.action.label, onClick: opts.action.onClick }
         : undefined,
     }
   }

   export const toast = {
     show: (opts: ShowOptions): string => toastManager.add(toAdd(opts)) as string,
     dismiss: (id: string) => toastManager.close(id),
     dismissByKey: (key: string) => toastManager.close(key),
     info: (message: string, description?: string) =>
       toastManager.add({ title: message, description, type: 'info' }),
     success: (message: string, description?: string) =>
       toastManager.add({ title: message, description, type: 'success' }),
     warning: (message: string, description?: string) =>
       toastManager.add({ title: message, description, type: 'warning' }),
     error: (message: string, description?: string) =>
       toastManager.add({ title: message, description, type: 'error' }),
   }
   ```

   Note: `toast.update` and `toast.has` existed in the old store but are NOT used at any call site outside toast-store itself — do not include them.

3. **Update `routes/__root.tsx`** — replace `<Toaster />` (from `sonner.tsx`) with `<ToastProvider>` + `<AnchoredToastProvider>` from `@/components/ui/toast`. Position: `bottom-right`.

4. **Delete `components/ui/sonner.tsx`** — it's replaced by the CossUI installation.

5. **Remove `sonner` from `web/package.json`** dependencies (it's currently `"sonner": "^2.0.7"`). Run `pnpm install` afterwards to update the lockfile.

6. **Run `pnpm --filter web tsc --noEmit`** — fix any TypeScript errors before committing.

7. **Commit** with message: `feat(toast): migrate to CossUI toast, replace Sonner`

---

## Task 2: Strip notifications system + migrate git-branch-manager

**Scope:** `web/src/` directory. Depends on Task 1 completing first.

### Steps

1. **Delete these files entirely:**
   - `features/window/components/notifications-sidebar.tsx`
   - `features/layout/contexts/toast-context.tsx`

2. **Update `features/layout/config/item-order.ts`** — remove `'notifications'` from `FOOTER_TRAILING_ITEM_IDS`:
   ```ts
   // Before:
   export const FOOTER_TRAILING_ITEM_IDS: string[] = ['notifications', 'settings']
   // After:
   export const FOOTER_TRAILING_ITEM_IDS: string[] = ['settings']
   ```

3. **Update `features/layout/utils/sidebar-pane-utils.ts`**:
   - Remove `'notifications'` from the `SidebarView` union type
   - Remove `'notifications'` from the `EDGE_SIDEBAR_VIEWS` set

4. **Update `features/git/components/git-branch-manager.tsx`** — remove the `useToast()` context import/usage. Replace `showToast(...)` calls with `toast.show(...)` imported from `@/features/window/stores/toast-store`. The `showToast` shape matches `toast.show` exactly.

5. **Grep for any remaining imports of the deleted files** and fix them:
   ```bash
   grep -r "notifications-sidebar\|toast-context\|NotificationsPane\|NotificationsTrigger\|useToast\b" web/src --include="*.ts" --include="*.tsx"
   ```

6. **Run `pnpm --filter web tsc --noEmit`** — fix any TypeScript errors.

7. **Run tests** covering changed files:
   ```bash
   pnpm --filter web test --run -- features/git/components/git-branch-manager
   ```

8. **Commit** with message: `chore(toast): strip notifications system, migrate git-branch-manager`
