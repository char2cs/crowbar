#!/usr/bin/env python3
"""Checks the crate layer table of docs/rust-style.md (M16, M17) over the Cargo.toml files of a root.

Usage: check-layers.py LAYERS ROOT [--complete]
LAYERS lists `crate layer kind` per line (kind: logic, gpui or binary). Every dependency on a listed
crate must point to a lower layer; a logic crate never depends on gpui or a gpui crate. With --complete, a crate
missing from LAYERS is an error too. Prints `path:line: [cargo-layer] message` per finding.
"""
import os
import re
import sys
import tomllib

SKIP_DIRS = {"target", "vendor", "node_modules"}
DEP_TABLES = ("dependencies", "dev-dependencies", "build-dependencies")


def read_layers(path):
    layers = {}
    for raw in open(path, encoding="utf-8"):
        line = raw.split("#", 1)[0].split()
        if len(line) == 3:
            layers[line[0]] = (int(line[1].lstrip("L")), line[2])
    return layers


def manifests(root):
    for here, dirs, files in os.walk(root):
        dirs[:] = sorted(d for d in dirs if d not in SKIP_DIRS)
        if "Cargo.toml" in files and os.path.abspath(here) != os.path.abspath(root):
            yield os.path.join(here, "Cargo.toml")


def dependencies(manifest):
    tables = [manifest.get(name, {}) for name in DEP_TABLES]
    for target in manifest.get("target", {}).values():
        tables += [target.get(name, {}) for name in DEP_TABLES]
    for table in tables:
        for key, spec in table.items():
            yield spec.get("package", key) if isinstance(spec, dict) else key


def line_of(text, name):
    pattern = re.compile(r"^\s*" + re.escape(name) + r"\s*(=|\.)", re.M)
    found = pattern.search(text)
    return text.count("\n", 0, found.start()) + 1 if found else 1


def findings(path, root, layers, complete):
    text = open(path, encoding="utf-8").read()
    manifest = tomllib.loads(text)
    crate = manifest.get("package", {}).get("name")
    rel = os.path.relpath(path, root)
    if crate is None:
        return
    if crate not in layers:
        if complete:
            yield f"{rel}:1: [cargo-layer] crate {crate} has no layer: add it to the layer table (docs/rust-style.md M17, tools/scripts/rust-layers.txt)"
        return
    level, kind = layers[crate]
    for dep in sorted(set(dependencies(manifest)) - {crate}):
        gpui_dep = dep.startswith("gpui") or (dep in layers and layers[dep][1] == "gpui")
        if kind == "logic" and gpui_dep:
            yield f"{rel}:{line_of(text, dep)}: [cargo-layer] {crate} is a logic crate and never depends on gpui or a gpui crate ({dep})"
        if dep in layers and layers[dep][0] >= level:
            how = "the same layer" if layers[dep][0] == level else "a higher layer"
            yield f"{rel}:{line_of(text, dep)}: [cargo-layer] {crate} (L{level}) depends on {dep} (L{layers[dep][0]}), {how}: dependencies point down"


def main(argv):
    complete = "--complete" in argv
    layers_file, root = [arg for arg in argv if arg != "--complete"][:2]
    layers = read_layers(layers_file)
    found = [line for path in manifests(root) for line in findings(path, root, layers, complete)]
    if found:
        print("\n".join(found))
    return 1 if found else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
