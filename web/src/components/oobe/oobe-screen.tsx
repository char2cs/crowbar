import { useState, useEffect } from 'react'
import { Navigate } from '@tanstack/react-router'
import { AnimatePresence, domAnimation, LazyMotion, m } from 'framer-motion'
import {
  ArrowClockwise,
  ArrowRight,
  CheckCircle,
  CircleNotchIcon,
  Warning,
  XCircle,
} from '@phosphor-icons/react'
import { CrowbarWordmark } from '@/components/ui/crowbar-wordmark'
import { Button } from '@/components/ui/button'
import { ImportProjectModal } from '@/components/projects/import-project-modal'
import { SidebarToastOverlay } from '@/components/layout/sidebar-toast-overlay'
import { useProjectStore } from '@/core/entities/active-project'
import { selectProjects, useSelect } from '@/core/entities/select'
import { landingFor } from '@/components/layout/shell-landing'
import { fetchPrerequisites } from '@/lib/api'
import type { Project, Prerequisites } from '@/lib/types'
import { OOBE_TAGLINE, OobeGradient } from './oobe-frame'

type OobeStep = 'presentation' | 'prerequisites' | 'add-project'

// ─── Prerequisite check row ──────────────────────────────────────────────────

type CheckStatus = 'loading' | 'ok' | 'warn' | 'error'

function CheckRow({
  label,
  description,
  status,
  order,
}: {
  label: string
  description: string
  status: CheckStatus
  /** The row's place in the list: rows come in one after another. */
  order: number
}) {
  return (
    <m.div
      initial={{ opacity: 0, x: -8 }}
      animate={{ opacity: 1, x: 0 }}
      transition={{ duration: 0.3, ease: 'easeOut', delay: order * 0.35 }}
      className="flex items-center justify-between gap-4 py-3"
    >
      <div>
        <p className="text-sm font-medium text-white">{label}</p>
        <p className="text-xs text-white/50">{description}</p>
      </div>
      <div className="flex shrink-0 items-center gap-1.5">
        {status === 'loading' && <CircleNotchIcon className="size-5 animate-spin text-white/40" />}
        {status === 'ok' && <CheckCircle weight="fill" className="size-5 text-emerald-400" />}
        {status === 'warn' && <Warning weight="fill" className="size-5 text-amber-400" />}
        {status === 'error' && <XCircle weight="fill" className="size-5 text-red-400" />}
      </div>
    </m.div>
  )
}

// ─── Prerequisites step ──────────────────────────────────────────────────────

function prereqStatus(
  check: { installed: boolean; authed?: boolean } | undefined,
  isProvider: boolean,
  otherProviderOk: boolean,
): CheckStatus {
  if (check === undefined) return 'loading'
  if (isProvider) {
    if (check.authed) return 'ok'
    return otherProviderOk ? 'warn' : 'error'
  }
  return check.installed ? 'ok' : 'error'
}

function PrerequisitesStep({
  prereqs,
  onContinue,
  onRetry,
}: {
  prereqs: Prerequisites | null
  onContinue: () => void
  onRetry: () => void
}) {
  const checked = prereqs !== null
  const ghOk = !!prereqs?.gh.authed
  const glabOk = !!prereqs?.glab.authed

  const gitStatus = prereqStatus(prereqs?.git, false, false)
  const ghStatus = prereqStatus(prereqs?.gh, true, glabOk)
  const glabStatus = prereqStatus(prereqs?.glab, true, ghOk)

  const canContinue =
    prereqs !== null && prereqs.git.installed && (prereqs.gh.authed || prereqs.glab.authed)

  const blockedReason = !checked
    ? null
    : !prereqs?.git.installed
      ? 'Git is required. Install it from git-scm.com.'
      : !prereqs?.gh.authed && !prereqs?.glab.authed
        ? 'Authenticate with at least one provider: run gh auth login or glab auth login.'
        : null

  return (
    <div className="flex flex-col gap-2">
      <div>
        <h2 className="font-heading text-2xl font-normal text-white">System check</h2>
        <p className="mt-0.5 text-xs text-white/50">
          Making sure everything's in place before you start.
        </p>
      </div>

      <div className="mt-2 divide-y divide-white/10">
        <CheckRow
          label="Git"
          description={
            !checked
              ? 'Checking…'
              : prereqs?.git.installed
                ? (prereqs.git.version ?? 'Installed')
                : 'Not found — install from git-scm.com'
          }
          status={gitStatus}
          order={0}
        />
        <CheckRow
          label="GitHub CLI (gh)"
          description={
            !checked
              ? 'Checking…'
              : prereqs?.gh.authed
                ? 'Authenticated'
                : prereqs?.gh.installed
                  ? 'Not signed in — run: gh auth login'
                  : 'Not installed — optional, enables GitHub features'
          }
          status={ghStatus}
          order={1}
        />
        <CheckRow
          label="GitLab CLI (glab)"
          description={
            !checked
              ? 'Checking…'
              : prereqs?.glab.authed
                ? 'Authenticated'
                : prereqs?.glab.installed
                  ? 'Not signed in — run: glab auth login'
                  : 'Not installed — optional, enables GitLab features'
          }
          status={glabStatus}
          order={2}
        />
      </div>

      <AnimatePresence>
        {blockedReason && (
          // Opacity-only: the strings here are short and fixed (see
          // `blockedReason` above), so a height tween buys nothing visually
          // over a fade — and it avoids animating a layout property (perf).
          <m.p
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            className="text-xs text-red-300"
          >
            {blockedReason}
          </m.p>
        )}
      </AnimatePresence>

      <div className="mt-4 flex gap-2">
        {checked && !canContinue && (
          <Button
            variant="ghost"
            size="sm"
            className="text-white/60 hover:text-white"
            onClick={onRetry}
          >
            <ArrowClockwise className="mr-1.5 size-3.5" />
            Check again
          </Button>
        )}
        <Button
          disabled={!canContinue}
          className="ml-auto rounded-full bg-white text-black hover:bg-white/90 disabled:opacity-40"
          onClick={onContinue}
        >
          Continue
          <ArrowRight className="ml-1.5 size-4" />
        </Button>
      </div>
    </div>
  )
}

// ─── Add project step ────────────────────────────────────────────────────────

function AddProjectStep({ onChooseFolder }: { onChooseFolder: () => void }) {
  return (
    <div className="flex flex-col gap-6">
      <div>
        <h2 className="font-heading text-2xl font-normal text-white">Add your first space</h2>
        <p className="mt-3 text-sm leading-relaxed text-white/60">
          A <span className="text-white">space</span> is the place where your repositories live.
          They don't have to be inside the space's folder — but it's a natural home for them.
        </p>
        <p className="mt-2 text-sm leading-relaxed text-white/60">
          Spaces are also where cross-repo plans, chats, and context get saved. Think of it as the
          shared memory for everything you're building.
        </p>
      </div>

      <Button
        className="w-full rounded-full bg-white text-black hover:bg-white/90"
        onClick={onChooseFolder}
      >
        Choose a folder
        <ArrowRight className="ml-1.5 size-4" />
      </Button>
    </div>
  )
}

// ─── Root component ──────────────────────────────────────────────────────────

export function OobeScreen() {
  const [step, setStep] = useState<OobeStep>('presentation')
  const [prereqs, setPrereqs] = useState<Prerequisites | null>(null)
  const [retryCount, setRetryCount] = useState(0)
  const [importOpen, setImportOpen] = useState(false)
  const projects = useSelect(selectProjects)
  const activeId = useProjectStore((s) => s.activeProjectId)

  useEffect(() => {
    if (step !== 'prerequisites') return
    let cancelled = false
    setPrereqs(null)
    fetchPrerequisites()
      .then((data) => {
        if (!cancelled) setPrereqs(data)
      })
      .catch(() => {
        if (cancelled) return
        // Surface failures as "not installed" so the UI can still guide the user
        setPrereqs({
          git: { installed: false },
          gh: { installed: false, authed: false },
          glab: { installed: false, authed: false },
        })
      })
    return () => {
      cancelled = true
    }
  }, [step, retryCount])

  // The import lands when its project is listed; the list leaves this screen, as it does the app's.
  function handleImport(project: Project) {
    useProjectStore.getState().setActiveProject(project.id)
    setImportOpen(false)
  }

  // Whoever added it (this dialog, the CLI, another client): a listed space ends the first run.
  const landing = landingFor(projects, activeId)
  if (landing && landing.to !== '/oobe') return <Navigate {...landing} replace />

  return (
    // Single LazyMotion boundary for the whole screen: it covers OobeGradient,
    // CheckRow, and PrerequisitesStep's m.* usage too, since they all render as
    // descendants here. Only enter/exit animations are used -> domAnimation.
    <LazyMotion features={domAnimation}>
      <div className="relative flex h-screen items-center justify-center overflow-hidden">
        <div data-tauri-drag-region className="absolute inset-x-0 top-0 z-20 h-8" />
        <OobeGradient />

        <>
          {/* ── Step 1: Presentation (full-screen, no card) ── */}
          <AnimatePresence>
            {step === 'presentation' && (
              <m.div
                key="presentation"
                initial={{ opacity: 0 }}
                animate={{ opacity: 1 }}
                exit={{ opacity: 0, transition: { duration: 0.25 } }}
                className="absolute inset-0 z-10 flex flex-col items-center justify-center gap-8 text-center"
              >
                <CrowbarWordmark className="w-[min(360px,44vw)] text-white" />
                <m.p
                  initial={{ opacity: 0, y: 6 }}
                  animate={{ opacity: 1, y: 0 }}
                  transition={{ delay: 0.15 }}
                  className="max-w-xs text-base text-white/70"
                >
                  {OOBE_TAGLINE}
                </m.p>
                <m.div
                  initial={{ opacity: 0, y: 6 }}
                  animate={{ opacity: 1, y: 0 }}
                  transition={{ delay: 0.25 }}
                >
                  <Button
                    className="rounded-full bg-white px-8 text-black hover:bg-white/90"
                    onClick={() => setStep('prerequisites')}
                  >
                    Get started with agents
                    <ArrowRight className="ml-1.5 size-4" />
                  </Button>
                </m.div>
              </m.div>
            )}
          </AnimatePresence>

          {/* ── Steps 2+: Glass card ── */}
          <AnimatePresence>
            {step !== 'presentation' && (
              <m.div
                key="card"
                initial={{ opacity: 0 }}
                animate={{ opacity: 1 }}
                exit={{ opacity: 0 }}
                transition={{ duration: 0.3 }}
                className="absolute inset-0 z-10 flex items-center justify-center p-6"
              >
                <div className="w-full max-w-md rounded-3xl border border-white/20 bg-white/10 p-10 shadow-2xl backdrop-blur-2xl">
                  {/* Logo pinned to top of card */}
                  <div className="mb-8 flex justify-center">
                    <CrowbarWordmark className="w-[min(180px,28vw)] text-white" />
                  </div>

                  {/* Step content cross-fades inside the card */}
                  <AnimatePresence mode="wait">
                    {step === 'prerequisites' && (
                      <m.div
                        key="prereqs"
                        initial={{ opacity: 0, y: 8 }}
                        animate={{ opacity: 1, y: 0 }}
                        exit={{ opacity: 0, y: -8 }}
                        transition={{ duration: 0.2 }}
                      >
                        <PrerequisitesStep
                          prereqs={prereqs}
                          onContinue={() => setStep('add-project')}
                          onRetry={() => setRetryCount((c) => c + 1)}
                        />
                      </m.div>
                    )}

                    {step === 'add-project' && (
                      <m.div
                        key="add-project"
                        initial={{ opacity: 0, y: 8 }}
                        animate={{ opacity: 1, y: 0 }}
                        exit={{ opacity: 0, y: -8 }}
                        transition={{ duration: 0.2 }}
                      >
                        <AddProjectStep onChooseFolder={() => setImportOpen(true)} />
                      </m.div>
                    )}
                  </AnimatePresence>
                </div>
              </m.div>
            )}
          </AnimatePresence>
        </>

        <ImportProjectModal
          open={importOpen}
          onOpenChange={setImportOpen}
          onImport={handleImport}
        />
        {/* No sidebar here: toasts float in the corner, as with the sidebar closed. */}
        <SidebarToastOverlay sidebarOpen={false} sidebarSide="left" />
      </div>
    </LazyMotion>
  )
}
