#!/usr/bin/env bash
#
# check-invariants.sh — enforces the §4.3 rules the compiler cannot.
#
# Run from `native/` (or anywhere: the script relocates to the workspace root).
# Exits non-zero with a specific message on the first class of violation found;
# it checks everything before exiting so one run reports every problem.
#
#   1. `crowbar-core` must not mention `gpui` (§4.3 rule 1, D2).
#   2. Every crate root outside `crowbar-platform` carries
#      `#![forbid(unsafe_code)]` (§4.3 rule 2).
#   3. Every `unsafe` construct in `crowbar-platform` has a `# Safety` doc
#      comment on its enclosing item (§4.2, §12).
#   4. No raw colour is constructed outside `crates/crowbar-ui/src/theme/`
#      (§4.3 rule 3, §6.1).
#
# Known limits, stated so nobody mistakes this for a parser: check 3 is a line
# scanner. It does not understand block comments, and a string literal
# containing the text `unsafe {` will be reported. Both failure modes are
# loud rather than silent, which is the correct direction for a gate. Check 4
# has its own scanner and its own stated limits; see its section.

set -euo pipefail

ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd -- "$ROOT"

status=0

fail() {
	printf 'FAIL  %s\n' "$1" >&2
	status=1
}

pass() {
	printf 'ok    %s\n' "$1"
}

# Crate roots: the default lib/bin targets plus any extra bins under src/bin/.
# `#![forbid(unsafe_code)]` at a crate root covers the whole crate, so roots are
# the complete set of places the attribute has to appear.
crate_roots() {
	local dir=$1 f
	for f in "$dir/src/lib.rs" "$dir/src/main.rs"; do
		[ -f "$f" ] && printf '%s\n' "$f"
	done
	if [ -d "$dir/src/bin" ]; then
		find "$dir/src/bin" -name '*.rs' -type f | sort
	fi
	return 0
}

# Every workspace member directory.
member_dirs() {
	local d
	for d in crates/*/; do
		[ -f "${d}Cargo.toml" ] && printf '%s\n' "${d%/}"
	done
	[ -f oracle/Cargo.toml ] && printf '%s\n' oracle
	return 0
}

# ---------------------------------------------------------------------------
# 1. crowbar-core must not depend on gpui.
# ---------------------------------------------------------------------------
# Comments are stripped first: this file explains the rule in prose, and a `#`
# comment is not a dependency, so stripping is correct rather than lenient.

core_manifest=crates/crowbar-core/Cargo.toml
if [ ! -f "$core_manifest" ]; then
	fail "rule 1: $core_manifest is missing"
elif sed 's/#.*//' "$core_manifest" | grep -qi 'gpui'; then
	fail "rule 1 (§4.3, D2): $core_manifest mentions gpui. crowbar-core is the"
	printf '      logic crate and must stay testable without a window. If a piece of\n' >&2
	printf '      logic seems to need gpui, it is not logic — split it.\n' >&2
	sed 's/#.*//' "$core_manifest" | grep -ni 'gpui' >&2 || true
else
	pass "rule 1: crowbar-core/Cargo.toml is free of gpui"
fi

# The same rule at the source level: a `use gpui::…` cannot compile without the
# manifest entry, but catching it here gives a message that names the rule.
if [ -d crates/crowbar-core/src ] &&
	grep -rn --include='*.rs' -E '(^|[^[:alnum:]_])gpui[[:space:]]*::' crates/crowbar-core/src >/dev/null 2>&1; then
	fail 'rule 1 (§4.3, D2): crowbar-core sources reference gpui::'
	grep -rn --include='*.rs' -E '(^|[^[:alnum:]_])gpui[[:space:]]*::' crates/crowbar-core/src >&2 || true
else
	pass 'rule 1: crowbar-core sources are free of gpui::'
fi

# ---------------------------------------------------------------------------
# 2. #![forbid(unsafe_code)] everywhere except crowbar-platform.
# ---------------------------------------------------------------------------

forbid_re='^[[:space:]]*#!\[forbid\(.*unsafe_code.*\)\]'
rule2_violations=0
checked_roots=0

while IFS= read -r dir; do
	[ -n "$dir" ] || continue
	while IFS= read -r root; do
		[ -n "$root" ] || continue
		checked_roots=$((checked_roots + 1))
		if [ "$dir" = crates/crowbar-platform ]; then
			# The one crate that is allowed unsafe must not forbid it, and must
			# deny the implicit-unsafe-body shortcut instead.
			if grep -qE "$forbid_re" "$root"; then
				fail "rule 2 (§4.3): $root has #![forbid(unsafe_code)], but"
				printf '      crowbar-platform is the crate that is *supposed* to hold the\n' >&2
				printf '      unsafe. Move the code that needs it, or drop the attribute.\n' >&2
				rule2_violations=$((rule2_violations + 1))
			fi
			if ! grep -qE '^[[:space:]]*#!\[deny\(.*unsafe_op_in_unsafe_fn.*\)\]' "$root"; then
				fail "rule 2 (§4.3): $root is missing #![deny(unsafe_op_in_unsafe_fn)]"
				rule2_violations=$((rule2_violations + 1))
			fi
		elif ! grep -qE "$forbid_re" "$root"; then
			fail "rule 2 (§4.3): $root is missing #![forbid(unsafe_code)]"
			rule2_violations=$((rule2_violations + 1))
		fi
	done <<<"$(crate_roots "$dir")"
done <<<"$(member_dirs)"

if [ "$checked_roots" -eq 0 ]; then
	fail 'rule 2: found no crate roots at all — is this the native/ workspace?'
elif [ "$rule2_violations" -eq 0 ]; then
	pass "rule 2: all $checked_roots crate roots carry the right unsafe attribute"
fi

# ---------------------------------------------------------------------------
# 3. Every unsafe construct in crowbar-platform is proved.
# ---------------------------------------------------------------------------
# The scanner tracks the doc-comment block attached to the most recent item and
# reports any `unsafe` reached while that block lacks a `# Safety` heading.
# Passes vacuously while the crate is empty; it is written for when it is not.

platform_src=crates/crowbar-platform/src
if [ ! -d "$platform_src" ]; then
	fail "rule 3: $platform_src is missing"
else
	unsafe_report=$(
		find "$platform_src" -name '*.rs' -type f | sort | while IFS= read -r f; do
			awk -v file="$f" '
			function is_item(l,   t) {
				t = l
				sub(/^[[:space:]]+/, "", t)
				while (1) {
					if (sub(/^pub[[:space:]]*\([^)]*\)[[:space:]]+/, "", t)) continue
					if (sub(/^pub[[:space:]]+/, "", t)) continue
					if (sub(/^default[[:space:]]+/, "", t)) continue
					if (sub(/^async[[:space:]]+/, "", t)) continue
					if (sub(/^unsafe[[:space:]]+/, "", t)) continue
					if (sub(/^extern[[:space:]]+"[^"]*"[[:space:]]+/, "", t)) continue
					if (sub(/^extern[[:space:]]+/, "", t)) continue
					break
				}
				return (t ~ /^(fn|impl|trait|mod|static|const|type|struct|enum|union)[[:space:](<:]/)
			}
			function has_unsafe(l) {
				return (l ~ /(^|[^[:alnum:]_])unsafe[[:space:]]*\{/) ||
				       (l ~ /(^|[^[:alnum:]_])unsafe[[:space:]]+(fn|impl|trait|extern)([^[:alnum:]_]|$)/) ||
				       (l ~ /(^|[^[:alnum:]_])unsafe[[:space:]]*$/)
			}
			BEGIN { doc = ""; item_proved = 0 }
			{
				trimmed = $0
				sub(/^[[:space:]]+/, "", trimmed)

				# Outer doc comment: accumulates onto the pending item.
				if (trimmed ~ /^\/\/\//) { doc = doc "\n" trimmed; next }
				# Inner doc (module-level) never proves an item.
				if (trimmed ~ /^\/\/!/) { doc = ""; next }
				# Attributes and blank lines may sit between a doc block and its
				# item without breaking the association.
				if (trimmed ~ /^#!?\[/ || trimmed == "") { next }
				# Ordinary comments are neither code nor documentation.
				if (trimmed ~ /^\/\//) { next }

				code = $0
				sub(/[[:space:]]*\/\/[^"]*$/, "", code)

				if (is_item(code)) {
					item_proved = (doc ~ /#[[:space:]]*Safety/)
					doc = ""
				} else {
					doc = ""
				}

				if (has_unsafe(code) && !item_proved) {
					printf "%s:%d: unsafe without a `# Safety` proof on the enclosing item:%s\n", file, NR, $0
				}
			}
		' "$f"
		done
	)
	if [ -n "$unsafe_report" ]; then
		fail 'rule 3 (§4.2, §12): unproved unsafe in crowbar-platform'
		printf '%s\n' "$unsafe_report" >&2
		printf '      Every unsafe construct needs a `# Safety` doc comment on its\n' >&2
		printf '      enclosing item that actually discharges the obligation: which\n' >&2
		printf '      selector, to what class, on which thread, with what lifetime\n' >&2
		printf '      guarantee on every pointer crossing the boundary.\n' >&2
	else
		pass 'rule 3: every unsafe in crowbar-platform carries a # Safety proof'
	fi
fi

# ---------------------------------------------------------------------------
# 4. No raw colour construction outside the theme.
# ---------------------------------------------------------------------------
# Sealing the token newtype is necessary and NOT sufficient, which is the part
# that is easy to get wrong. `crowbar-ui` re-exports gpui (`pub use gpui;`) —
# it has to, because §4.2 gives the leaf view crates the framework *through*
# the design system — so `crowbar_ui::gpui::rgb(0x1e1e1e)` is in scope
# everywhere and `.bg(rgb(…))` on a raw gpui element never touches `Theme`. A
# private field on `Color` does nothing about that: the bypass does not go
# through `Color` at all.
#
# So: no `rgb(`, `rgba(`, `hsl(`, `hsla(`, `Hsla {` or `Rgba {` anywhere except
# `crates/crowbar-ui/src/theme/`, which is where the tokens are minted from
# `theme.css` and where `Color::mix` reseals a derived one.
#
# False positives are handled rather than tolerated:
#
#   * the theme directory itself is exempt — that is the whole point of it;
#   * `#[cfg(test)]` blocks are exempt, tracked by brace depth, because a unit
#     test asserting on a colour is not a component painting one, and so are
#     the `tests/` integration-test directories for the same reason;
#   * comments and string literals are stripped before matching, so the prose
#     in this file's own subject matter — and a `"rgba(…)"` in a message — does
#     not trip it;
#   * the match requires a non-identifier character before the name, so
#     `Srgba { … }` (crowbar-core's own sRGB type, which carries no gpui) and a
#     hypothetical `srgb(` are not colour construction and are not reported;
#   * `fn f() -> Hsla {` and `impl T for Rgba {` name the type rather than
#     building one, and are not reported — a body that does build one still is.
#
# Raw strings (`r"…"`, `r#"…"#`, `br#"…"#`, `cr#"…"#`, and the multi-line form)
# *are* understood, as of P1.5. They were a stated limit and the limit bit:
# `crowbar-driver`'s schema tests embed a JSON fixture in an `r##"…"##`, whose
# braces miscounted the depth, which ended the `#[cfg(test)]` skip early and
# reported three colour literals inside a test module as violations. A gate that
# cries wolf is a gate people learn to skip, so the scanner was taught the
# syntax rather than the rule loosened — the *rule* is untouched, and the
# adversarial check below still fails on a real literal placed after a raw
# string in the same file.
#
# Stated limit that remains: character literals. A `'{'` still miscounts brace
# depth. It fails loud, which is the right direction for a gate.

theme_dir=crates/crowbar-ui/src/theme
if [ ! -d "$theme_dir" ]; then
	fail "rule 4: $theme_dir is missing — where are the tokens minted?"
else
	color_report=$(
		find crates oracle -name '*.rs' -type f 2>/dev/null |
			grep -v "^$theme_dir/" |
			grep -v '/tests/' |
			sort |
			while IFS= read -r f; do
				awk -v file="$f" '
				# Where a raw string opens at position i, if one does: the length
				# of its `r`/`br`/`cr` + `#`* + `"` prefix, else 0. The character
				# before the prefix must not continue an identifier, or the `r` of
				# `for` would open one.
				function raw_open(line, i,   j, prefix, before, candidate) {
					prefix = substr(line, i, 1)
					if (prefix == "b" || prefix == "c") {
						if (substr(line, i + 1, 1) != "r") return 0
						j = i + 2
					} else if (prefix == "r") {
						j = i + 1
					} else {
						return 0
					}
					if (i > 1) {
						before = substr(line, i - 1, 1)
						if (before ~ /[A-Za-z0-9_]/) return 0
					}
					candidate = "\""
					while (substr(line, j, 1) == "#") {
						candidate = candidate "#"
						j++
					}
					if (substr(line, j, 1) != "\"") return 0
					raw_close = candidate
					return j - i + 1
				}

				# Strip block comments, line comments, string literals and raw
				# strings, so that only code reaches the matcher.
				function clean(line,   out, i, n, ch, ch2, c, opened, at) {
					out = ""
					i = 1
					n = length(line)
					while (i <= n) {
						if (in_raw) {
							at = index(substr(line, i), raw_close)
							if (at == 0) break
							i = i + at - 1 + length(raw_close)
							in_raw = 0
							continue
						}
						ch = substr(line, i, 1)
						ch2 = substr(line, i, 2)
						if (in_block) {
							if (ch2 == "*/") { in_block = 0; i += 2 } else { i++ }
							continue
						}
						if (ch2 == "/*") { in_block = 1; i += 2; continue }
						if (ch2 == "//") { break }
						opened = raw_open(line, i)
						if (opened > 0) {
							in_raw = 1
							i += opened
							continue
						}
						if (ch == "\"") {
							i++
							while (i <= n) {
								c = substr(line, i, 1)
								if (c == "\\") { i += 2; continue }
								if (c == "\"") { i++; break }
								i++
							}
							continue
						}
						out = out ch
						i++
					}
					return out
				}
				BEGIN {
					in_block = 0; in_raw = 0; raw_close = ""
					depth = 0; pending = 0; skipping = 0
				}
				{
					code = clean($0)

					# Brace depth. Inline rather than in a helper: awk evaluates
					# a regex literal passed as an argument against $0 and hands
					# the function a 0 or a 1, which counts nothing.
					braces = code
					opens = gsub(/\{/, "&", braces)
					braces = code
					closes = gsub(/\}/, "&", braces)

					before = depth
					depth = before + opens - closes

					if (skipping) {
						if (depth <= skip_depth) skipping = 0
						next
					}
					if (code ~ /#\[cfg\((all\()?[[:space:]]*test[,)]/) {
						pending = 1
						next
					}
					if (pending) {
						if (depth > before) { skipping = 1; skip_depth = before }
						pending = 0
						next
					}

					# A brace after `Hsla` is only a struct literal in expression
					# position. `fn f() -> Hsla {` and `impl T for Rgba {` name
					# the type, they do not build one; neutralise both before
					# matching, or every function that *returns* a colour is a
					# violation and the rule becomes noise people learn to skip.
					probe = code
					gsub(/->[[:space:]]*([A-Za-z0-9_]+[[:space:]]*::[[:space:]]*)*(Hsla|Rgba)[[:space:]]*\{/, "-> T {", probe)
					gsub(/(^|[^A-Za-z0-9_])for[[:space:]]+([A-Za-z0-9_]+[[:space:]]*::[[:space:]]*)*(Hsla|Rgba)[[:space:]]*\{/, " for T {", probe)

					if (probe ~ /(^|[^A-Za-z0-9_])(rgba?|hsla?)[[:space:]]*\(/ ||
					    probe ~ /(^|[^A-Za-z0-9_])(Hsla|Rgba)[[:space:]]*\{/) {
						printf "%s:%d:%s\n", file, NR, $0
					}
				}
			' "$f"
			done
	)
	if [ -n "$color_report" ]; then
		fail 'rule 4 (§4.3 rule 3, §6.1): raw colour construction outside the theme'
		printf '%s\n' "$color_report" >&2
		printf '      A colour literal at a call site is exactly the bypass the sealed\n' >&2
		printf '      token newtype cannot catch: `crowbar-ui` re-exports gpui, so\n' >&2
		printf '      `rgb(0x1e1e1e)` is reachable from every view crate and never\n' >&2
		printf '      touches `Theme`. Read the token instead — `theme.background`,\n' >&2
		printf '      `theme.border` — or, if the colour is genuinely derived, derive\n' >&2
		printf '      it with `Color::mix` from one that is.\n' >&2
	else
		pass 'rule 4: no raw colour construction outside crowbar-ui/src/theme/'
	fi
fi

if [ "$status" -ne 0 ]; then
	printf '\ncheck-invariants.sh: FAILED\n' >&2
fi
exit "$status"
