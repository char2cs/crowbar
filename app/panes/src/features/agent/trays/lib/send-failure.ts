import type { OutboxItem } from '@/features/agent/stores/lib/outbox'

export interface SendFailureText {
  headline: string
  hint: string
  /** A resend would fail the same way, so only Discard is offered. */
  final: boolean
}

/** A failed send in plain, provider-neutral words; the raw error is never shown. */
export function describeSendFailure(item: Pick<OutboxItem, 'code'>): SendFailureText {
  switch (item.code) {
    case 'session_lost':
      return {
        headline: 'The session was lost',
        hint: 'Start a new conversation to send this.',
        final: true,
      }
    case 'prompt_submit_unsupported':
      return {
        headline: 'This agent can’t take typed messages here',
        hint: 'Use its terminal instead.',
        final: true,
      }
    case 'too_large':
      return {
        headline: 'Your message is too large to send',
        hint: 'Shorten it and try again.',
        final: true,
      }
    default:
      return {
        headline: 'Your message was not sent',
        hint: 'Check the connection and try again.',
        final: false,
      }
  }
}
