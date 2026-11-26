import { bootPane } from './pane-boot'
import { BufferPaneMount } from './buffer-pane-mount'
import { DocumentPaneHost } from './document-pane-host'
import { followDocumentText } from './document-session'

const { client, container } = bootPane('core.editor')
followDocumentText(client)
new BufferPaneMount(container, client, <DocumentPaneHost client={client} />).start()
client.register()
