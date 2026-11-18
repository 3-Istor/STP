#!/usr/bin/env bash
# Installe les outils épinglés de scripts/tools.env dans .tools/bin (rien de global). ADR 0039.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"
mkdir -p .tools/bin
while IFS='=' read -r name mod; do
  [[ -z "$name" || "$name" == \#* ]] && continue
  echo "• $name ($mod)"
  GOBIN="$PWD/.tools/bin" go install "$mod"
done < scripts/tools.env
echo "Outils dans .tools/bin (ajouté au PATH par scripts/verify.sh)."
