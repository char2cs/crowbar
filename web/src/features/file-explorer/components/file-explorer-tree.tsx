// Short-path compat shim: re-export only the component so this file stays
// Fast-Refresh-safe (only-export-components). Import the tree's non-component
// helpers/types from the real module under file-explorer/file-explorer/.
export { FileExplorerTree } from '../file-explorer/components/file-explorer-tree'
