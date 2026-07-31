'use client'

import { mergeProps } from '@base-ui/react/merge-props'
import { useRender } from '@base-ui/react/use-render'
import { Children } from 'react'
import type React from 'react'
import { cn } from '@/lib/utils'

/*
 * Oracle anchors (native/oracle/ANCHORS.md). One per slot, and **no
 * `data-oracle-content-sized` anywhere**: the card is `w-full` under a
 * `max-w-sm` and every slot is a stretched block or grid item, so not one of
 * these boxes sizes to its own text.
 *
 * `data-oracle-line-sized` is on `CardTitle` alone, and conditionally, for the
 * reason `label.tsx` gives: v1.6 makes the declaration valid only on an anchor
 * that carries a `font`, and the differ refuses — by name — a document that
 * declares it on a box painting no text. A `<CardTitle />` with no children is
 * such a box. The native half withholds it in exactly the same case.
 *
 * **This surface deliberately declares no anchor set in `oracleSurfaceScope`.**
 * v1.8 permits one only where the set is a property of the surface, and here
 * every slot is optional and the call site chooses — so the set is a property of
 * the cell, exactly as `git-status-row`'s is.
 */

export function Card({
  className,
  render,
  ...props
}: useRender.ComponentProps<'div'>): React.ReactElement {
  const defaultProps = {
    className: cn(
      'relative flex flex-col rounded-2xl border bg-card not-dark:bg-clip-padding text-card-foreground shadow-xs/5 before:pointer-events-none before:absolute before:inset-0 before:rounded-[calc(var(--radius-2xl)-1px)] before:shadow-[0_1px_--theme(--color-black/4%)] dark:before:shadow-[0_-1px_--theme(--color-white/6%)]',
      className,
    ),
    'data-oracle-id': 'card',
    'data-slot': 'card',
  }

  return useRender({
    defaultTagName: 'div',
    props: mergeProps<'div'>(defaultProps, props),
    render,
  })
}

export function CardHeader({
  className,
  render,
  ...props
}: useRender.ComponentProps<'div'>): React.ReactElement {
  const defaultProps = {
    className: cn(
      'grid auto-rows-min grid-rows-[auto_auto] items-start gap-1.5 p-6 in-[[data-slot=card]:has(>[data-slot=card-panel])]:pb-4 has-data-[slot=card-action]:grid-cols-[1fr_auto]',
      className,
    ),
    'data-oracle-id': 'card-header',
    'data-slot': 'card-header',
  }

  return useRender({
    defaultTagName: 'div',
    props: mergeProps<'div'>(defaultProps, props),
    render,
  })
}

export function CardTitle({
  className,
  render,
  ...props
}: useRender.ComponentProps<'div'>): React.ReactElement {
  const paintsARun = Children.count(props.children) > 0
  const defaultProps = {
    className: cn('font-semibold text-lg leading-none', className),
    ...(paintsARun ? { 'data-oracle-line-sized': 'true' } : {}),
    'data-oracle-id': 'card-title',
    'data-slot': 'card-title',
  }

  return useRender({
    defaultTagName: 'div',
    props: mergeProps<'div'>(defaultProps, props),
    render,
  })
}

function CardPanel({
  className,
  render,
  ...props
}: useRender.ComponentProps<'div'>): React.ReactElement {
  const defaultProps = {
    className: cn(
      'flex-1 p-6 in-[[data-slot=card]:has(>[data-slot=card-header]:not(.border-b))]:pt-0 in-[[data-slot=card]:has(>[data-slot=card-footer]:not(.border-t))]:pb-0',
      className,
    ),
    'data-oracle-id': 'card-panel',
    'data-slot': 'card-panel',
  }

  return useRender({
    defaultTagName: 'div',
    props: mergeProps<'div'>(defaultProps, props),
    render,
  })
}

export { CardPanel as CardContent }
