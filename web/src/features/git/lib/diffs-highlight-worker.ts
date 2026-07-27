// Web Worker entry for @pierre/diffs' Shiki highlighting pool.
//
// The package ships the worker as an export (`@pierre/diffs/worker/worker.js`),
// but a `new Worker(new URL(...), import.meta.url)` call can only name a path
// Vite can resolve RELATIVE to the calling module — a bare package specifier is
// left untouched and 404s at runtime. This one-line module is that relative
// path: importing the package worker for its side effects (it installs the
// message handler on `self`) and nothing else.
import '@pierre/diffs/worker/worker.js'
