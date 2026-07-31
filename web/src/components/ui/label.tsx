'use client'

import { mergeProps } from '@base-ui/react/merge-props'
import { useRender } from '@base-ui/react/use-render'
import { Children } from 'react'
import type React from 'react'
import { cn } from '@/lib/utils'

/**
 * Carries `data-oracle-content-sized` always and `data-oracle-line-sized` only
 * when it has children.
 *
 * The box is `inline-flex` with no authored width or height, so both of
 * `native/oracle/ANCHORS.md` v1.5 and v1.6 hold: its width is the run's
 * max-content width, and its height *is* the run's line box. Unlike `badge` and
 * `kbd`, nothing here authors a height, which is the test v1.6 actually states.
 *
 * **The `line-sized` declaration is conditional**, and that is not caution: v1.6
 * makes it valid only on an anchor that carries a `font`, and the differ refuses
 * — by name — a document that declares it on a box painting no text. A `<Label>`
 * with no children is such a box, so the declaration is withheld there. The
 * native half drops it in exactly the same case
 * (`crowbar_ui::components::label::Label::render`), which is what keeps the two
 * extractors from disagreeing inside a silence.
 */
export function Label({
  className,
  render,
  ...props
}: useRender.ComponentProps<'label'>): React.ReactElement {
  const paintsARun = Children.count(props.children) > 0
  const defaultProps = {
    className: cn(
      'inline-flex items-center gap-2 font-medium text-base/4.5 text-foreground sm:text-sm/4',
      className,
    ),
    'data-oracle-content-sized': 'true',
    ...(paintsARun ? { 'data-oracle-line-sized': 'true' } : {}),
    'data-oracle-id': 'label',
    'data-slot': 'label',
  }

  return useRender({
    defaultTagName: 'label',
    props: mergeProps<'label'>(defaultProps, props),
    render,
  })
}
