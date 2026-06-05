#!/bin/bash
set -euo pipefail

cd /home/playground

cat > Rux.toml << RUXEOF
[Package]
Name = "playground"
Version = "0.1.0"
Authors = ["Playground User"]
Description = "Rux Playground submission"

[Dependencies]
Std = "*"
Linux = "*"
RUXEOF

mkdir -p Src
cp /workspace/Main.rux Src/

rux install >/dev/null 2>&1 || true

if ! rux build >/dev/null 2>/tmp/rux_err; then
    cat /tmp/rux_err >&2
    exit 1
fi

exec ./Bin/Debug/playground
