# Crowbar website

The landing page for Crowbar, served at <https://crowbar.char2cs.net>. A single
static Astro page; no framework components.

```sh
bun install
bun run dev      # http://localhost:4321
bun run check    # astro check (types)
bun run build    # static output in dist/
```

CI runs `check` and `build` on any pull request that touches `website/`.
