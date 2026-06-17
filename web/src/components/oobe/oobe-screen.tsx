import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { ShaderGradient, ShaderGradientCanvas } from '@shadergradient/react'
import {
  Empty,
  EmptyHeader,
  EmptyTitle,
  EmptyDescription,
  EmptyContent,
} from '@/components/ui/empty'
import { CrowbarWordmark } from '@/components/ui/crowbar-wordmark'
import { Button } from '@/components/ui/button'
import { ImportProjectModal } from '@/components/projects/import-project-modal'
import { importProjectAndSync } from '@/lib/store/projects'
import type { Project } from '@/lib/types'

export function OobeScreen() {
  const [importOpen, setImportOpen] = useState(false)
  const navigate = useNavigate()

  function handleImport(project: Project) {
    importProjectAndSync(project)
    setImportOpen(false)
    void navigate({ to: '/' })
  }

  return (
    <div className="relative flex h-screen flex-col overflow-hidden">
      {/* Animated gradient background */}
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

      {/* Content */}
      <Empty className="relative z-10">
        <CrowbarWordmark className="mb-2 w-48 text-white" />
        <EmptyHeader>
          <EmptyTitle className="text-white">Open a project folder</EmptyTitle>
          <EmptyDescription className="text-white/70">Choose a local directory to get started.</EmptyDescription>
        </EmptyHeader>
        <EmptyContent>
          <Button
            className="w-full rounded-full bg-white text-black hover:bg-white/90"
            onClick={() => setImportOpen(true)}
          >
            Choose folder
          </Button>
        </EmptyContent>
      </Empty>

      <ImportProjectModal
        open={importOpen}
        onOpenChange={setImportOpen}
        onImport={handleImport}
      />
    </div>
  )
}
