# Harnais V2 — plan et contrat commun des lots

Demande (Brian, 17/11/2026) : une usine logicielle « vraiment poussée » — contrôles locaux et CI, sécurité, IA,
skills courtes — en reprenant ce que l'équipe a fait sur `3-Istor/expense-io` (`docs/harness.md`) et
`3-Istor/sdi-harness` (doctrine, `decision-autonomy.md`, `gates.md`), adapté à STP (Go + React), et en suivant
l'état de l'art récent. Branche : `v2-harnais` (partie de `v2/implementation`).

## Cadre

Böckeler (Thoughtworks, *Harness Engineering*, avril 2026), repris par expense-io :
- **guides** (avant l'action : `AGENTS.md`, skills, relecteurs) et **capteurs** (après : tests, linters, relecture) ;
- **calculés** (déterministes, rapides) d'abord, **inférés** (jugement d'un modèle) ensuite : un relecteur IA ne
  peut pas rendre vert ce que la moulinette refuse ;
- le rapide en local, le coûteux dans la CI et la nuit ; la CI confirme, elle ne découvre pas.

Anthropic, *How the Claude Code team does code review* (15/10/2026) : vérification avant PR (tests, build, lint,
puis sous-agent en contexte neuf), revue IA à sévérités **Important / Nit / Pre-existing**, constats filtrés,
**niveau de risque calculé sur le changement, pas sur l'auteur**, l'approbation d'un agent ne vaut jamais celle
d'un humain, **3 tours de revue au plus** puis on reconsidère l'approche, une classe de bug signalée est corrigée
partout.

## Niveaux de risque (contrat commun : `AGENTS.md`, `cmd/risk`, `CODEOWNERS` les appliquent tous)

| Niveau | Déclencheurs (chemins) | Humain |
|---|---|---|
| 0 Cosmétique | `docs/**` sauf `docs/adr/`, `*.md` hors racine, `web/src/**/*.css`, captures | aucun en direct |
| 1 Routine | code d'un seul domaine `internal/<d>/`, `web/src/**`, tests ajoutés, outil de dev | lit le résumé de preuves |
| 2 Sensible | `migrations/`, routes ou JSON de `internal/httpapi/` (contrat `/api/v1`), plusieurs domaines, `go.mod`/`web/package.json` (dépendance d'exécution), `Dockerfile`, `compose.yaml`, `internal/sampledata/`, `docs/adr/` | valide le plan / le diff de contrat avant |
| 3 Critique | règles métier non négociables : `internal/money/`, immuabilité des snapshots (`internal/snapshot/`, triggers SQL), calcul du coût (`internal/bom/cost*.go`, `totals.go`, `bridge.go`) ; harnais : `.github/`, `.githooks/`, `.claude/`, `AGENTS.md`, `CLAUDE.md`, `cmd/risk/`, `internal/archtest/`, `scripts/verify*`, `.golangci.yml`, `CODEOWNERS`, `SECURITY.md` ; test supprimé ou affaibli (`t.Skip`, `test.skip`, `.only`) | valide le plan, relit le diff sensible ; 2 relecteurs IA en contexte neuf |

Le plus élevé l'emporte ; on peut monter, jamais descendre ; un fichier inconnu = niveau 1.

## Lots

- **H1 — capteurs calculés** (Go/shell, sans nouvelle dépendance runtime) : `make verify LEVEL=fast|pr|nightly` +
  `verify-summary.md` ; `cmd/risk` ; `internal/archtest` ; inventaire des routes `/api/v1` figé (golden) ;
  contrôle de dérive de la doc des agents ; hooks git `.githooks/` (`make hooks`) ; CI durcie (actions épinglées par
  SHA, permissions minimales, dependency-review, gitleaks, osv-scanner, actionlint, SBOM, nightly avec mutation et
  historique des secrets, Scorecard hebdo) ; `CODEOWNERS`, `SECURITY.md`. ADR **0039**.
- **H2 — guides et capteurs inférés** (IA) : `AGENTS.md` réécrit en carte < 120 lignes (aucune règle perdue : le
  détail part dans `docs/agents/`), `.claude/settings.json` (refus, hooks), relecteurs en contexte neuf
  (`.claude/agents/`), skills courtes (`.claude/skills/`, 40–100 lignes), skill `security-audit` de Cloudflare
  (MIT, vendue avec attribution), `REVIEW.md` (sévérités), workflow de revue IA optionnel (sans clé : ne tourne
  pas). ADR **0040**.
- **Doc de synthèse** : `docs/harnais.md` (pour les slides), écrite après H1 et H2.
