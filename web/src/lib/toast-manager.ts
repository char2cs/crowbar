// Re-export toast managers so stores can import from lib/ without crossing the
// stores-must-not-import-from-components/ rule.
export { toastManager } from '@/components/ui/toast'
