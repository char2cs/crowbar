import { bootPane } from './pane-boot'
import { BufferPaneHost } from './buffer-pane-host'
import { BufferPaneMount } from './buffer-pane-mount'
import { openReviewBuffer, revealInActiveWorkspace } from './open-review-buffer'

const { client, container } = bootPane('core.review')
new BufferPaneMount(
  container,
  client,
  <BufferPaneHost client={client} open={openReviewBuffer} />,
).start()
client.onReviewReveal(revealInActiveWorkspace)
client.register()
