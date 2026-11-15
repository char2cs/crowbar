import { bootPane } from './pane-boot'
import { BufferPaneMount } from './buffer-pane-mount'
import { openEditorBuffer } from './open-editor-buffer'

const { client, container } = bootPane('core.editor')
new BufferPaneMount(container, client, openEditorBuffer).start()
client.register()
