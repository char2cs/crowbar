import { describe, it, expect, beforeEach } from 'vitest'
import { useSidebarNavStore } from '@/features/layout/stores/sidebar-nav'

beforeEach(() => {
  useSidebarNavStore.getState().reset()
})

describe('useSidebarNavStore', () => {
  it('starts with empty stack', () => {
    expect(useSidebarNavStore.getState().stack).toHaveLength(0)
  })

  it('push adds a screen to the stack', () => {
    useSidebarNavStore.getState().push({ id: 'a', title: 'A', component: null })
    expect(useSidebarNavStore.getState().stack).toHaveLength(1)
    expect(useSidebarNavStore.getState().stack[0].id).toBe('a')
  })

  it('push with duplicate id is a no-op', () => {
    useSidebarNavStore.getState().push({ id: 'a', title: 'A', component: null })
    useSidebarNavStore.getState().push({ id: 'a', title: 'A2', component: null })
    expect(useSidebarNavStore.getState().stack).toHaveLength(1)
    expect(useSidebarNavStore.getState().stack[0].title).toBe('A')
  })

  it('pop removes the last screen', () => {
    useSidebarNavStore.getState().push({ id: 'a', title: 'A', component: null })
    useSidebarNavStore.getState().push({ id: 'b', title: 'B', component: null })
    useSidebarNavStore.getState().pop()
    expect(useSidebarNavStore.getState().stack).toHaveLength(1)
    expect(useSidebarNavStore.getState().stack[0].id).toBe('a')
  })

  it('pop on empty stack is a no-op', () => {
    expect(() => useSidebarNavStore.getState().pop()).not.toThrow()
    expect(useSidebarNavStore.getState().stack).toHaveLength(0)
  })

  it('reset clears the stack', () => {
    useSidebarNavStore.getState().push({ id: 'a', title: 'A', component: null })
    useSidebarNavStore.getState().push({ id: 'b', title: 'B', component: null })
    useSidebarNavStore.getState().reset()
    expect(useSidebarNavStore.getState().stack).toHaveLength(0)
  })
})
