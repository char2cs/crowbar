import { bootPane } from './pane-boot'
import { ConsolePaneMount } from './console-pane-mount'

const { client, container } = bootPane('core.console')
new ConsolePaneMount(container, client).start()
client.register()
