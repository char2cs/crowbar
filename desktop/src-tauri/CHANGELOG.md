# Changelog

## 0.1.0 (2026-09-04)


### Features

* **agent:** give agents Crowbar's own tools, over MCP ([#110](https://github.com/char2cs/crowbar/issues/110)) ([bb0bfe7](https://github.com/char2cs/crowbar/commit/bb0bfe7221b8c5165d5caefbfe89e1030deb1524))
* design language overhaul — chrome surfaces, tab pills, sidebar, and native browser pane ([#4](https://github.com/char2cs/crowbar/issues/4)) ([6d7e51f](https://github.com/char2cs/crowbar/commit/6d7e51f293a75135c24b1481692117d44d94afb3))
* terminal screen-model rebuild + editor hosting, workspace provisioning, provider/avatar features ([#19](https://github.com/char2cs/crowbar/issues/19)) ([9736908](https://github.com/char2cs/crowbar/commit/9736908370b8529627f8afcd2e3e33e39a3ded85))
* v0 backend spec suite, architecture reference, and scaffold corrections ([#1](https://github.com/char2cs/crowbar/issues/1)) ([23588a2](https://github.com/char2cs/crowbar/commit/23588a29710bf9a31b887e51c7efc4acac9a80b8))


### Bug Fixes

* audit follow-ups — teardown that doesn't depend on the peer, two missing timeouts, three daemon bugs ([#53](https://github.com/char2cs/crowbar/issues/53)) ([f78b7ff](https://github.com/char2cs/crowbar/commit/f78b7ffc180ebb56d0ec37195d4a2a9031494d8c))
* **ci:** auto-merge Dependabot patch/minor bumps and drop dead deps ([#133](https://github.com/char2cs/crowbar/issues/133)) ([6cb971f](https://github.com/char2cs/crowbar/commit/6cb971fff71ee83ccd6288f76b83fb72145026ed))
* daemon reliability, origin-sync, repo-scoped workspace data layer, and UI/editor polish ([#32](https://github.com/char2cs/crowbar/issues/32)) ([6addb3f](https://github.com/char2cs/crowbar/commit/6addb3fdd2dbe4f1a32979686fbdb71978aeff2a))
* **daemon:** stop the watchdog from killing a suspended-but-healthy backend (+ watcher efficiency) ([#51](https://github.com/char2cs/crowbar/issues/51)) ([9fcd5d0](https://github.com/char2cs/crowbar/commit/9fcd5d0112a7ce46c062aa1caaa0235baef25d54))
* **desktop:** anchor beforeBuildCommand's icon-compile path to repo root ([#20](https://github.com/char2cs/crowbar/issues/20)) ([8f86061](https://github.com/char2cs/crowbar/commit/8f860616ad668d8694e4d8b8fcfd431314299ebe))
* **desktop:** keep the macOS window blur alive when the window loses focus ([#127](https://github.com/char2cs/crowbar/issues/127)) ([cfd21ce](https://github.com/char2cs/crowbar/commit/cfd21cef2f81501e850c0032b20dda8465513031))
* **desktop:** the app leaks fds until it cannot dial the daemon — and then kills it ([#52](https://github.com/char2cs/crowbar/issues/52)) ([2884f7c](https://github.com/char2cs/crowbar/commit/2884f7ca9dc27c2e07b6c30cfeaf5b8f7c82f1eb))
* markdown preview scroll retention, worktree fork base, and the whole Dependabot queue ([#94](https://github.com/char2cs/crowbar/issues/94)) ([c8163af](https://github.com/char2cs/crowbar/commit/c8163afd8658ea6401e856afb2186aa4ddd759ba))


### Performance

* bare-metal frontend performance + React Doctor 100/100 + agent-chat lifecycle ([#75](https://github.com/char2cs/crowbar/issues/75)) ([495483b](https://github.com/char2cs/crowbar/commit/495483b53d3aeacbb4d0c1f35eb873d183331048))
* **diff:** windowed review surface, plus the fixes that piled up around it ([#106](https://github.com/char2cs/crowbar/issues/106)) ([580df68](https://github.com/char2cs/crowbar/commit/580df68588d288a38a9a43bb50f5f1fc172516e6))
* idle-CPU, daemon allocation & asynx snapshot O(1) + truthful agent spinner ([#77](https://github.com/char2cs/crowbar/issues/77)) ([c1113e4](https://github.com/char2cs/crowbar/commit/c1113e4252b20e2e70c74387ba5586580e3b4c17))
