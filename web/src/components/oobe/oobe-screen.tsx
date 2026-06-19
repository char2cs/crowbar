import { useState, useEffect } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { motion, AnimatePresence } from 'framer-motion'
import { ShaderGradient, ShaderGradientCanvas } from '@shadergradient/react'
import { CheckCircle, XCircle, Warning, ArrowRight, ArrowClockwise } from '@phosphor-icons/react'
import { Loader2 } from 'lucide-react'
import { CrowbarWordmark } from '@/components/ui/crowbar-wordmark'
import { Button } from '@/components/ui/button'
import { ImportProjectModal } from '@/components/projects/import-project-modal'
import { importProjectAndSync, useProjectStore } from '@/lib/store/projects'
import { fetchPrerequisites } from '@/lib/api'
import type { Project, Prerequisites } from '@/lib/types'

type OobeStep = 'presentation' | 'prerequisites' | 'add-project'

// ─── Gradient (extracted so JSX stays readable) ─────────────────────────────

function GradientBackground() {
  return (
    <div className="pointer-events-none absolute inset-0">
      <ShaderGradientCanvas style={{ width: '100%', height: '100%' }} fov={45} pixelDensity={1}>
        <ShaderGradient
          animate="on"
          brightness={0.8}
          cAzimuthAngle={270}
          cDistance={0.5}
          cPolarAngle={180}
          cameraZoom={15.09}
          color1="#73bfc4"
          color2="#ff810a"
          color3="#8da0ce"
          envPreset="city"
          grain="on"
          lightType="env"
          positionX={-0.1}
          positionY={0}
          positionZ={0}
          range="disabled"
          rangeEnd={40}
          rangeStart={0}
          reflection={0.4}
          rotationX={0}
          rotationY={130}
          rotationZ={70}
          shader="defaults"
          type="sphere"
          uAmplitude={3.2}
          uDensity={0.8}
          uFrequency={5.5}
          uSpeed={0.3}
          uStrength={0.3}
          uTime={0}
          wireframe={false}
        />
      </ShaderGradientCanvas>
    </div>
  )
}

// ─── Prerequisite check row ──────────────────────────────────────────────────

type CheckStatus = 'loading' | 'ok' | 'warn' | 'error'

function CheckRow({
  label,
  description,
  status,
  visible,
}: {
  label: string
  description: string
  status: CheckStatus
  visible: boolean
}) {
  return (
    <motion.div
      initial={{ opacity: 0, x: -8 }}
      animate={visible ? { opacity: 1, x: 0 } : { opacity: 0, x: -8 }}
      transition={{ duration: 0.3, ease: 'easeOut' }}
      className="flex items-center justify-between gap-4 py-3"
    >
      <div>
        <p className="text-sm font-medium text-white">{label}</p>
        <p className="text-xs text-white/50">{description}</p>
      </div>
      <div className="flex shrink-0 items-center gap-1.5">
        {status === 'loading' && <Loader2 className="size-5 animate-spin text-white/40" />}
        {status === 'ok' && <CheckCircle weight="fill" className="size-5 text-emerald-400" />}
        {status === 'warn' && <Warning weight="fill" className="size-5 text-amber-400" />}
        {status === 'error' && <XCircle weight="fill" className="size-5 text-red-400" />}
      </div>
    </motion.div>
  )
}

// ─── Prerequisites step ──────────────────────────────────────────────────────

function prereqStatus(
  check: { installed: boolean; authed?: boolean } | undefined,
  visible: boolean,
  isProvider: boolean,
  otherProviderOk: boolean,
): CheckStatus {
  if (!visible || check === undefined) return 'loading'
  if (isProvider) {
    if (check.authed) return 'ok'
    return otherProviderOk ? 'warn' : 'error'
  }
  return check.installed ? 'ok' : 'error'
}

function PrerequisitesStep({
  prereqs,
  visible,
  onContinue,
  onRetry,
}: {
  prereqs: Prerequisites | null
  visible: { git: boolean; gh: boolean; glab: boolean }
  onContinue: () => void
  onRetry: () => void
}) {
  const allVisible = visible.git && visible.gh && visible.glab
  const ghOk = !!prereqs?.gh.authed
  const glabOk = !!prereqs?.glab.authed

  const gitStatus = prereqStatus(prereqs?.git, visible.git, false, false)
  const ghStatus = prereqStatus(prereqs?.gh, visible.gh, true, glabOk)
  const glabStatus = prereqStatus(prereqs?.glab, visible.glab, true, ghOk)

  const canContinue =
    allVisible &&
    prereqs !== null &&
    prereqs.git.installed &&
    (prereqs.gh.authed || prereqs.glab.authed)

  const blockedReason = !allVisible
    ? null
    : !prereqs?.git.installed
      ? 'Git is required. Install it from git-scm.com.'
      : !prereqs?.gh.authed && !prereqs?.glab.authed
        ? 'Authenticate with at least one provider: run gh auth login or glab auth login.'
        : null

  return (
    <div className="flex flex-col gap-2">
      <div>
        <h2 className="text-base font-semibold text-white">System check</h2>
        <p className="mt-0.5 text-xs text-white/50">
          Making sure everything's in place before you start.
        </p>
      </div>

      <div className="mt-2 divide-y divide-white/10">
        <CheckRow
          label="Git"
          description={
            !visible.git
              ? 'Checking…'
              : prereqs?.git.installed
                ? prereqs.git.version ?? 'Installed'
                : 'Not found — install from git-scm.com'
          }
          status={gitStatus}
          visible={visible.git}
        />
        <CheckRow
          label="GitHub CLI (gh)"
          description={
            !visible.gh
              ? 'Checking…'
              : prereqs?.gh.authed
                ? 'Authenticated'
                : prereqs?.gh.installed
                  ? 'Not signed in — run: gh auth login'
                  : 'Not installed — optional, enables GitHub features'
          }
          status={ghStatus}
          visible={visible.gh}
        />
        <CheckRow
          label="GitLab CLI (glab)"
          description={
            !visible.glab
              ? 'Checking…'
              : prereqs?.glab.authed
                ? 'Authenticated'
                : prereqs?.glab.installed
                  ? 'Not signed in — run: glab auth login'
                  : 'Not installed — optional, enables GitLab features'
          }
          status={glabStatus}
          visible={visible.glab}
        />
      </div>

      <AnimatePresence>
        {blockedReason && (
          <motion.p
            initial={{ opacity: 0, height: 0 }}
            animate={{ opacity: 1, height: 'auto' }}
            exit={{ opacity: 0, height: 0 }}
            className="text-xs text-red-300"
          >
            {blockedReason}
          </motion.p>
        )}
      </AnimatePresence>

      <div className="mt-4 flex gap-2">
        {allVisible && !canContinue && (
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
        <h2 className="text-base font-semibold text-white">Add your first project</h2>
        <p className="mt-3 text-sm leading-relaxed text-white/60">
          A <span className="text-white">project</span> is the place where your repositories live.
          They don't have to be inside the project folder — but it's a natural home for them.
        </p>
        <p className="mt-2 text-sm leading-relaxed text-white/60">
          Projects are also where cross-repo plans, chats, and context get saved. Think of it as
          the shared memory for everything you're building.
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
  const [visible, setVisible] = useState({ git: false, gh: false, glab: false })
  const [retryCount, setRetryCount] = useState(0)
  const [importOpen, setImportOpen] = useState(false)
  const navigate = useNavigate()

  useEffect(() => {
    if (step !== 'prerequisites') return
    let cancelled = false
    setPrereqs(null)
    setVisible({ git: false, gh: false, glab: false })

    const timers: ReturnType<typeof setTimeout>[] = []

    fetchPrerequisites()
      .then((data) => {
        if (cancelled) return
        setPrereqs(data)
        timers.push(setTimeout(() => setVisible((v) => ({ ...v, git: true })), 300))
        timers.push(setTimeout(() => setVisible((v) => ({ ...v, gh: true })), 650))
        timers.push(setTimeout(() => setVisible((v) => ({ ...v, glab: true })), 1000))
      })
      .catch(() => {
        if (cancelled) return
        // Surface failures as "not installed" so the UI can still guide the user
        setPrereqs({ git: { installed: false }, gh: { installed: false, authed: false }, glab: { installed: false, authed: false } })
        timers.push(setTimeout(() => setVisible((v) => ({ ...v, git: true })), 300))
        timers.push(setTimeout(() => setVisible((v) => ({ ...v, gh: true })), 650))
        timers.push(setTimeout(() => setVisible((v) => ({ ...v, glab: true })), 1000))
      })

    return () => {
      cancelled = true
      timers.forEach(clearTimeout)
    }
  }, [step, retryCount])

  function handleImport(project: Project) {
    importProjectAndSync(project)
    useProjectStore.getState().setActiveProject(project.id)
    setImportOpen(false)
    void navigate({ to: '/' })
  }

  return (
    <div className="relative flex h-screen items-center justify-center overflow-hidden">
      <div data-tauri-drag-region className="absolute inset-x-0 top-0 z-20 h-8" />
      <GradientBackground />

      <>
        {/* ── Step 1: Presentation (full-screen, no card) ── */}
        <AnimatePresence>
          {step === 'presentation' && (
            <motion.div
              key="presentation"
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              exit={{ opacity: 0, transition: { duration: 0.25 } }}
              className="absolute inset-0 z-10 flex flex-col items-center justify-center gap-8 text-center"
            >
              <CrowbarWordmark className="w-[min(360px,44vw)] text-white" />
              <motion.p
                initial={{ opacity: 0, y: 6 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ delay: 0.15 }}
                className="max-w-xs text-base text-white/70"
              >
                The IDE where agents do the heavy lifting.
              </motion.p>
              <motion.div
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
              </motion.div>
            </motion.div>
          )}
        </AnimatePresence>

        {/* ── Steps 2+: Glass card ── */}
        <AnimatePresence>
          {step !== 'presentation' && (
            <motion.div
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
                  <motion.div
                    key="prereqs"
                    initial={{ opacity: 0, y: 8 }}
                    animate={{ opacity: 1, y: 0 }}
                    exit={{ opacity: 0, y: -8 }}
                    transition={{ duration: 0.2 }}
                  >
                    <PrerequisitesStep
                      prereqs={prereqs}
                      visible={visible}
                      onContinue={() => setStep('add-project')}
                      onRetry={() => setRetryCount((c) => c + 1)}
                    />
                  </motion.div>
                )}

                {step === 'add-project' && (
                  <motion.div
                    key="add-project"
                    initial={{ opacity: 0, y: 8 }}
                    animate={{ opacity: 1, y: 0 }}
                    exit={{ opacity: 0, y: -8 }}
                    transition={{ duration: 0.2 }}
                  >
                    <AddProjectStep onChooseFolder={() => setImportOpen(true)} />
                  </motion.div>
                )}
              </AnimatePresence>
            </div>
            </motion.div>
          )}
        </AnimatePresence>
      </>

      <ImportProjectModal open={importOpen} onOpenChange={setImportOpen} onImport={handleImport} quick />
    </div>
  )
}
