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

if [ "${DUMP_ASM:-0}" = "1" ]; then
    rux build --dump-asm >/dev/null 2>/tmp/rux_err || { cat /tmp/rux_err >&2; exit 1; }
    full="/tmp/full.asm"
    cat Temp/Asm/out.asm > "$full"
    printf '===USER_ASM_START===\n'
    sed -n '/; ── Main ─/,$ p' "$full"
    printf '\n===USER_ASM_END===\n'
    printf '===FULL_ASM_START===\n'
    cat "$full"
    printf '\n===FULL_ASM_END===\n'
else
    rux build >/dev/null 2>/tmp/rux_err || { cat /tmp/rux_err >&2; exit 1; }
    exec ./Bin/Debug/playground
fi
