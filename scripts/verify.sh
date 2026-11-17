#!/usr/bin/env bash
# Moulinette locale (ADR 0039) : les mêmes capteurs calculés qu'en CI, par niveau de coût.
#   fast    : pre-commit, < 1 min, sans base ni réseau
#   pr      : avant de pousser, = la CI (base locale si DATABASE_URL joignable)
#   nightly : pr + fuzzing, mutation, secrets sur tout l'historique
# Chaque étape finit passed, failed ou not_run (avec la raison) : une étape non lancée n'est
# jamais comptée verte. Résumé : .verify/verify-summary.md (lu par l'humain au niveau 1).
set -uo pipefail

LEVEL="${1:-${LEVEL:-fast}}"
case "$LEVEL" in fast|pr|nightly) ;; *) echo "usage: $0 fast|pr|nightly" >&2; exit 2 ;; esac
cd "$(git rev-parse --show-toplevel)"
export PATH="$PWD/.tools/bin:$PATH"   # outils épinglés de make tools d'abord
mkdir -p .verify
SUMMARY=.verify/verify-summary.md
LOG=.verify/verify.log
: >"$LOG"
ROWS=()
FAILED=0

have() { command -v "$1" >/dev/null 2>&1; }

# step <nom> <commande…> : lance, chronomètre, journalise.
step() {
  local name="$1"; shift
  local start=$SECONDS
  printf '• %s : ' "$name"
  if { echo "### $name"; "$@"; } >>"$LOG" 2>&1; then
    ROWS+=("| $name | passed | $((SECONDS - start)) s | |"); echo "ok"
  else
    ROWS+=("| $name | **failed** | $((SECONDS - start)) s | voir \`$LOG\` |"); echo "ÉCHEC"; FAILED=1
  fi
}

# skip <nom> <raison> : étape requise à ce niveau mais impossible ici, dite comme telle.
skip() { ROWS+=("| $1 | not_run | | $2 |"); printf '• %s : non lancé (%s)\n' "$1" "$2"; }

gofmt_clean() { local out; out=$(gofmt -l cmd internal migrations 2>&1); [ -z "$out" ] || { echo "$out"; return 1; }; }
web_ready() { [ -d web/node_modules ]; }
db_up() { [ -n "${DATABASE_URL:-}" ] && have pg_isready && pg_isready -q -d "$DATABASE_URL"; }

# --- fast : ce qui attrape 80 % des erreurs en quelques secondes ------------------------
step "gofmt" gofmt_clean
step "go vet" go vet ./...
step "frontières d'architecture" go test -count=1 ./internal/archtest/
step "contrat des routes /api/v1" go test -count=1 -run TestRoutesContract ./internal/httpapi/
step "niveaux de risque + doc agents" go test -count=1 ./cmd/risk/
if have gitleaks; then
  step "secrets (index)" gitleaks git --staged --redact --no-banner
else
  skip "secrets (index)" "gitleaks absent : make tools"
fi
if web_ready; then step "oxlint (web)" npm --prefix web run --silent lint; else skip "oxlint (web)" "web/node_modules absent : npm ci"; fi

# --- pr : la CI en local ------------------------------------------------------------------
if [ "$LEVEL" != fast ]; then
  step "go mod tidy à jour" go mod tidy -diff
  step "code sqlc à jour" bash -c 'go tool sqlc diff'
  step "golangci-lint (dont gosec)" go tool golangci-lint run ./...
  if db_up; then
    step "tests Go -race (intégration incluse)" go test -race -count=1 ./...
  else
    step "tests Go -race (sans base)" go test -race -count=1 ./...
    skip "tests d'intégration PostgreSQL" "DATABASE_URL injoignable : la CI les lance"
  fi
  step "govulncheck" go tool govulncheck ./...
  if have osv-scanner; then step "osv-scanner (go.sum, package-lock)" osv-scanner scan source --lockfile go.mod --lockfile web/package-lock.json
  else skip "osv-scanner" "absent : make tools"; fi
  if have actionlint; then step "actionlint (workflows)" actionlint
  else skip "actionlint" "absent : make tools"; fi
  if web_ready; then
    step "tests unitaires web" npm --prefix web run --silent test:unit
    step "build web (tsc + vite)" npm --prefix web run --silent build
    step "npm audit (prod, high)" npm --prefix web audit --omit=dev --audit-level=high
  else
    skip "web (tests, build, audit)" "web/node_modules absent : npm ci"
  fi
fi

# --- nightly : le coûteux ----------------------------------------------------------------
if [ "$LEVEL" = nightly ]; then
  step "fuzzing money (60 s)" go test -run '^$' -fuzz FuzzParse -fuzztime 60s ./internal/money/
  if have gremlins; then
    # Cliquet : 83,9 % mesuré le 17/11/2026 ; on ne descend jamais sous 80 (ADR 0039).
    step "mutation money (efficacité ≥ 80 %)" gremlins unleash --threshold-efficacy 80 ./internal/money
    step "mutation bom (mesure)" gremlins unleash --workers 2 ./internal/bom
  else
    skip "mutation (money, bom)" "gremlins absent : make tools"
  fi
  if have gitleaks; then step "secrets (tout l'historique)" gitleaks git --redact --no-banner
  else skip "secrets (tout l'historique)" "gitleaks absent : make tools"; fi
fi

# --- résumé -------------------------------------------------------------------------------
{
  echo "# Moulinette — niveau \`$LEVEL\` — $(git rev-parse --short HEAD)"
  echo
  echo "| Étape | Statut | Durée | Note |"
  echo "|---|---|---|---|"
  printf '%s\n' "${ROWS[@]}"
  echo
  go run ./cmd/risk 2>/dev/null || echo "_Niveau de risque non calculé (pas de base git de comparaison)._"
} >"$SUMMARY"

echo
if [ "$FAILED" -ne 0 ]; then
  echo "Moulinette ROUGE ($LEVEL). Détail : $LOG — résumé : $SUMMARY"; exit 1
fi
echo "Moulinette verte ($LEVEL). Résumé : $SUMMARY"
