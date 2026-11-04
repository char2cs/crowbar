import { beforeEach, describe, expect, it } from 'vitest'
import { useMobileScreenStore } from '@/features/window/stores/mobile-screen-store'

beforeEach(() => {
  useMobileScreenStore.setState({ screen: 'chat', chatListOpen: false, filesOpen: false, ack: 0 })
})

describe('useMobileScreenStore', () => {
  it('starts on the chat with the chat list closed', () => {
    const { screen, chatListOpen } = useMobileScreenStore.getState()
    expect(screen).toBe('chat')
    expect(chatListOpen).toBe(false)
  })

  it('pushes the IDE over the chat, and back returns to the chat', () => {
    useMobileScreenStore.getState().open('ide')
    expect(useMobileScreenStore.getState().screen).toBe('ide')
    useMobileScreenStore.getState().back()
    expect(useMobileScreenStore.getState().screen).toBe('chat')
  })

  // The IDE covers the chat, so the chat list it was opened from is put away.
  it('closes the chat list when the IDE is pushed', () => {
    useMobileScreenStore.getState().setChatListOpen(true)
    useMobileScreenStore.getState().open('ide')
    expect(useMobileScreenStore.getState().chatListOpen).toBe(false)
  })
  // The file explorer is a layer of the IDE: back puts it away before leaving the IDE.
  it('opens the IDE’s files over it, and back closes them first', () => {
    const screens = useMobileScreenStore.getState()
    screens.open('ide')
    screens.setFilesOpen(true)
    expect(useMobileScreenStore.getState().filesOpen).toBe(true)
    useMobileScreenStore.getState().back()
    expect(useMobileScreenStore.getState()).toMatchObject({ screen: 'ide', filesOpen: false })
    useMobileScreenStore.getState().back()
    expect(useMobileScreenStore.getState().screen).toBe('chat')
  })

  it('keeps the highest intent the chrome has been answered for', () => {
    const screens = useMobileScreenStore.getState()
    screens.acknowledge(2)
    expect(useMobileScreenStore.getState().ack).toBe(2)
  })
})
