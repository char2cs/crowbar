import * as React from 'react'

import { SlateElement } from 'platejs/static'

import { CalloutBody, calloutClassName } from './callout-content'

export function CalloutElementStatic({
  attributes,
  children,
  className,
  ...props
}: React.ComponentProps<typeof SlateElement>) {
  return (
    <SlateElement
      className={calloutClassName(className)}
      style={{
        backgroundColor: props.element.backgroundColor as React.CSSProperties['backgroundColor'],
      }}
      attributes={attributes}
      {...props}
    >
      <CalloutBody icon={props.element.icon as string | undefined}>{children}</CalloutBody>
    </SlateElement>
  )
}
