import type { StateCreator } from 'zustand'
import type { WorkspaceState } from '../workspace-store.types'
import type { FlowDefinition } from '@/features/workflow/types/workflow'
import type { ChatMessage } from '@/lib/types'

export interface WorkflowActions {
  setFlowDefinition(def: FlowDefinition): void
  setCurrentStep(stepId: string): void
  markStepCompleted(stepId: string): void
  /** Append a message to the chat for a given step. */
  addChatMessage(stepId: string, message: ChatMessage): void
  /** Append a partial streaming chunk to the last assistant message, or create one. */
  appendStreamingChunk(stepId: string, chunk: string, streamingId: string): void
  /** Finalise the streaming message (replace temp id). */
  finaliseStreamingMessage(stepId: string, streamingId: string, finalId: string): void
  /** Ensure the step has messages; seeds with initialMessages if empty. */
  initChatStep(stepId: string, initialMessages: ChatMessage[]): void
  setSendingForStep(stepId: string, sending: boolean): void
}

export interface WorkflowSlice {
  flowDefinition: FlowDefinition | null
  currentStepId: string | null
  /** Chat message history keyed by stepId. */
  chatMessages: Record<string, ChatMessage[]>
  /** Which steps have an in-flight streaming response. */
  sendingByStep: Record<string, boolean>
  workflowActions: WorkflowActions
}

export const createWorkflowSlice: StateCreator<
  WorkspaceState,
  [['zustand/immer', never]],
  [],
  WorkflowSlice
> = (set) => ({
  flowDefinition: null,
  currentStepId: null,
  chatMessages: {},
  sendingByStep: {},

  workflowActions: {
    setFlowDefinition(def) {
      set(state => {
        state.flowDefinition = def
        if (!state.currentStepId) {
          state.currentStepId = def.steps[0]?.id ?? null
        }
      })
    },
    setCurrentStep(stepId) {
      set(state => { state.currentStepId = stepId })
    },
    markStepCompleted(stepId) {
      set(state => {
        const step = state.flowDefinition?.steps.find(s => s.id === stepId)
        if (step) step.isCompleted = true
      })
    },
    addChatMessage(stepId, message) {
      set(state => {
        if (!state.chatMessages[stepId]) state.chatMessages[stepId] = []
        state.chatMessages[stepId].push(message)
      })
    },
    appendStreamingChunk(stepId, chunk, streamingId) {
      set(state => {
        if (!state.chatMessages[stepId]) state.chatMessages[stepId] = []
        const msgs = state.chatMessages[stepId]
        const last = msgs[msgs.length - 1]
        if (last?.id === streamingId) {
          last.content += chunk
        } else {
          msgs.push({
            id: streamingId,
            role: 'assistant',
            content: chunk,
            authorName: 'Claude',
            authorInitials: '✦',
            modelName: 'Sonnet 4.6',
            timestamp: 'just now',
          })
        }
      })
    },
    finaliseStreamingMessage(stepId, streamingId, finalId) {
      set(state => {
        const msgs = state.chatMessages[stepId]
        if (!msgs) return
        const msg = msgs.find(m => m.id === streamingId)
        if (msg) msg.id = finalId
      })
    },
    initChatStep(stepId, initialMessages) {
      set(state => {
        if (!state.chatMessages[stepId] || state.chatMessages[stepId].length === 0) {
          state.chatMessages[stepId] = initialMessages
        }
      })
    },
    setSendingForStep(stepId, sending) {
      set(state => { state.sendingByStep[stepId] = sending })
    },
  },
})
