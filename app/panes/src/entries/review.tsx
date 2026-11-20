import { bootPane } from './pane-boot'
import { BufferPaneMount } from './buffer-pane-mount'
import { openReviewBuffer, revealInActiveWorkspace } from './open-review-buffer'

const { client, container } = bootPane('core.review')
new BufferPaneMount(container, client, openReviewBuffer).start()
client.onReviewReveal(revealInActiveWorkspace)
client.register()
