import { describe, it, expect, beforeEach } from 'vitest'
import { useZoomStore } from '@/features/window/stores/zoom-store'

const RESET_STATE = { zoom: 1, editorZoomLevel: 1, terminalZoomLevel: 1 }

describe('zoom-store', () => {
  beforeEach(() => {
    useZoomStore.setState(RESET_STATE)
  })

  it('starts at 1x for chat, editor and terminal', () => {
    const state = useZoomStore.getState()
    expect(state.zoom).toBe(1)
    expect(state.editorZoomLevel).toBe(1)
    expect(state.terminalZoomLevel).toBe(1)
  })

  it('zoomIn steps zoom up by 0.1', () => {
    useZoomStore.getState().actions.zoomIn()
    expect(useZoomStore.getState().zoom).toBeCloseTo(1.1)
  })

  it('zoomOut steps zoom down by 0.1', () => {
    useZoomStore.getState().actions.zoomOut()
    expect(useZoomStore.getState().zoom).toBeCloseTo(0.9)
  })

  it('clamps zoomIn at 3x', () => {
    useZoomStore.setState({ zoom: 3 })
    useZoomStore.getState().actions.zoomIn()
    expect(useZoomStore.getState().zoom).toBe(3)
  })

  it('clamps zoomOut at 0.3x', () => {
    useZoomStore.setState({ zoom: 0.3 })
    useZoomStore.getState().actions.zoomOut()
    expect(useZoomStore.getState().zoom).toBe(0.3)
  })

  it('setZoom sets an exact value', () => {
    useZoomStore.getState().actions.setZoom(1.7)
    expect(useZoomStore.getState().zoom).toBe(1.7)
  })

  it('resetZoom resets zoom, editorZoomLevel and terminalZoomLevel together', () => {
    useZoomStore.setState({ zoom: 2, editorZoomLevel: 1.5, terminalZoomLevel: 0.5 })
    useZoomStore.getState().actions.resetZoom()
    expect(useZoomStore.getState().zoom).toBe(1)
    expect(useZoomStore.getState().editorZoomLevel).toBe(1)
    expect(useZoomStore.getState().terminalZoomLevel).toBe(1)
  })

  it('resetZoom does not disturb zoom levels it did not touch mid-sequence', () => {
    useZoomStore.getState().actions.setEditorZoomLevel(1.3)
    useZoomStore.getState().actions.setTerminalZoomLevel(0.8)
    useZoomStore.getState().actions.zoomIn()
    expect(useZoomStore.getState()).toMatchObject({
      zoom: 1.1,
      editorZoomLevel: 1.3,
      terminalZoomLevel: 0.8,
    })
  })
})
