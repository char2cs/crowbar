import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import type { FlowStateDefinition } from '@/lib/types'

interface WorkspaceStepTabsProps {
  states: FlowStateDefinition[]
  currentStep: string
  onStepChange: (step: string) => void
}

function StepDot({ state }: { state: 'done' | 'active' | 'pending' }) {
  return (
    <span className={
      'h-1.5 w-1.5 rounded-full flex-shrink-0 ' +
      (state === 'done' ? 'bg-primary/40' : state === 'active' ? 'bg-primary' : 'bg-muted-foreground/30')
    } />
  )
}

export function WorkspaceStepTabs({ states, currentStep, onStepChange }: WorkspaceStepTabsProps) {
  const activeIdx = states.findIndex(s => s.name === currentStep)
  return (
    <Tabs value={currentStep} onValueChange={(v) => onStepChange(v as string)}>
      <TabsList className="h-10 w-full justify-start gap-0 rounded-none border-b border-border bg-card px-4">
        {states.map((s, i) => (
          <div key={s.name} className="flex items-center">
            <TabsTrigger
              value={s.name}
              className="flex items-center gap-1.5 rounded-none border-b-2 border-transparent px-3 py-2 text-[13px] text-muted-foreground data-[state=active]:border-primary data-[state=active]:bg-transparent data-[state=active]:text-foreground data-[state=active]:shadow-none"
            >
              <StepDot state={i < activeIdx ? 'done' : i === activeIdx ? 'active' : 'pending'} />
              {s.label}
            </TabsTrigger>
            {i < states.length - 1 && (
              <span className="mx-0.5 text-[12px] text-muted-foreground/30">›</span>
            )}
          </div>
        ))}
      </TabsList>
    </Tabs>
  )
}
