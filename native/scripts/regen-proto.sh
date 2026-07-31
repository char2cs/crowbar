#!/usr/bin/env bash
#
# regen-proto.sh — the one command that reproduces `crowbar-proto`.
#
# Run from anywhere; the script relocates to the repo root.
#
#   native/scripts/regen-proto.sh
#
# It exists because the flags are not guessable. Item 0.5 shipped the generator
# and never ran it, and the output the crate was supposed to hold went missing
# for long enough that four crates depended on an empty package. Reconstructing
# `-daemon-root`, `-out-rust`, `-out-rust-tests` and `-manifest` from the tool's
# doc comment is exactly the step nobody does twice the same way, so it is
# written down here and nowhere else.
#
# What it does, in order:
#
#   1. builds `native/tools/protogen` (with -buildvcs=false; see below);
#   2. runs it over `api/`, emitting the Rust DTOs, the round-trip tests and
#      the JSON manifest;
#   3. enforces the §4.2 dependency contract on the emitted Rust — see below;
#   4. formats the result with rustfmt, because §4.3's rule 5 gates the whole
#      tree on `cargo fmt --check` and generated output is not exempt.
#
# It deliberately does NOT touch `web/`. protogen can emit the TypeScript
# declarations too (`-out-ts`), and regenerating them is a separate decision
# with a separate blast radius: the React app is the live reference for the
# Phase 1 parity gate, and churning its types perturbs the thing being measured.
# Add the flag when that gate is done, not before.
#
# -buildvcs=false is not optional here. Go's VCS stamping walks up out of
# `native/tools/protogen`, finds the enclosing repo, and fails the build with
# `error obtaining VCS status: exit status 128`. Environmental and pre-existing;
# it reproduces on a pristine checkout.

set -euo pipefail

ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
cd -- "$ROOT"

GENERATED="native/crates/crowbar-proto/src/generated"
TESTS="native/crates/crowbar-proto/tests"
MANIFEST="native/protogen.manifest.json"

# ---------------------------------------------------------------------------
# Toolchain.
# ---------------------------------------------------------------------------
# Rust installs to ~/.cargo/bin, which a non-login shell does not have on PATH.
# A missing tool must be a loud failure and not a silent skip: this script's
# whole job is to leave the tree in a state the gates accept, and a skipped
# rustfmt step leaves it in a state rule 5 rejects.

export PATH="$HOME/.cargo/bin:$PATH"

for tool in go rustfmt; do
	if ! command -v "$tool" >/dev/null 2>&1; then
		printf 'regen-proto: %s is not on PATH\n' "$tool" >&2
		exit 1
	fi
done

BIN="$(mktemp -t protogen)"
trap 'rm -f "$BIN"' EXIT

printf 'regen-proto: building protogen\n' >&2
(cd native/tools/protogen && go build -buildvcs=false -o "$BIN" .)

printf 'regen-proto: generating from ./api\n' >&2
"$BIN" \
	-daemon-root ./api \
	-out-rust "$GENERATED" \
	-out-rust-tests "$TESTS" \
	-manifest "$MANIFEST"

# ---------------------------------------------------------------------------
# The §4.2 dependency contract, enforced on the output.
# ---------------------------------------------------------------------------
# `crowbar-proto` may depend on `serde` and nothing else. protogen lowers an
# untyped Go payload — `any`, `map[string]any`, `json.RawMessage` — to
# `serde_json::Value`, which needs a second crate. Today that is exactly one
# emitted declaration: `gin.H`, gin's own map helper, which reaches the closure
# only because five handlers still answer with an untyped `gin.H` instead of a
# named DTO (`native/protogen.manifest.json`, category `untyped-payload`). No
# Crowbar DTO references it.
#
# Adding `serde_json` to make it compile is the wrong repair, and it is the one
# that would be reached for first: the crate is generated output whose whole
# value is that it is trivially reviewable, and the fix for an untyped payload
# is a named DTO on the Go side. So the module is dropped here, loudly, on every
# run — the regeneration stays a single reproducible command, and the pressure
# stays where the defect is.
#
# A module is only ever dropped when nothing else refers to it. If a real DTO
# ever grows an untyped field, the reference check below fails the run instead,
# because dropping that module would silently take a live type with it.

drop_uncarryable_modules() {
	local dropped=() f base
	for f in "$GENERATED"/*.rs; do
		base="$(basename "$f")"
		[ "$base" = mod.rs ] && continue
		grep -q 'serde_json' "$f" || continue
		dropped+=("$base")
	done
	if [ ${#dropped[@]} -eq 0 ]; then
		return 0
	fi

	local module refs
	for base in "${dropped[@]}"; do
		module="${base%.rs}"
		refs=$(grep -rln "super::${module}::" "$GENERATED" | grep -v "/${base}$" || true)
		if [ -n "$refs" ]; then
			printf 'regen-proto: FAILED — %s needs serde_json and is referenced by:\n' "$module" >&2
			printf '%s\n' "$refs" >&2
			printf '  §4.2 gives crowbar-proto serde and nothing else, so this module\n' >&2
			printf '  cannot be carried and cannot be dropped either. Fix it on the Go\n' >&2
			printf '  side: give the handler a named DTO instead of an untyped payload.\n' >&2
			exit 1
		fi
	done

	for base in "${dropped[@]}"; do
		module="${base%.rs}"
		rm -- "${GENERATED:?}/${base}"
		# Rewrite rather than sed -i in place: BSD and GNU sed disagree about
		# -i's argument, and this script runs on both.
		grep -v "^pub mod ${module};\$" "$GENERATED/mod.rs" >"$GENERATED/mod.rs.tmp"
		mv -- "$GENERATED/mod.rs.tmp" "$GENERATED/mod.rs"
		printf 'regen-proto: dropped module %s — it needs serde_json, which §4.2 forbids\n' \
			"$module" >&2
	done
}

drop_uncarryable_modules

# ---------------------------------------------------------------------------
# Formatting.
# ---------------------------------------------------------------------------
# rustfmt is invoked per file rather than through `cargo fmt`, which would walk
# the whole workspace and rewrite files this script does not own.

printf 'regen-proto: formatting\n' >&2
rustfmt --edition 2024 --quiet "$GENERATED"/*.rs "$TESTS"/generated_roundtrip.rs

printf 'regen-proto: %s modules, %s tests\n' \
	"$(find "$GENERATED" -name '*.rs' | wc -l | tr -d ' ')" \
	"$(grep -c '^fn ' "$TESTS/generated_roundtrip.rs" | tr -d ' ')" >&2
