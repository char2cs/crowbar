const TAB_HEIGHT_PX = 32
// The tab pill's clearance above and below: the old 4px top gap plus a third of it, rounded.
const TAB_GAP_PX = 5

/** Height of every top chrome row: pane headers, the view-tab strip, the sidebar header. */
export const CHROME_ROW_HEIGHT_PX = TAB_HEIGHT_PX + 2 * TAB_GAP_PX

/** Where the traffic lights sit so they centre in that row (the OS anchors them a hair high). */
export const TRAFFIC_LIGHT_Y_PX = Math.round(CHROME_ROW_HEIGHT_PX / 2) + 2
