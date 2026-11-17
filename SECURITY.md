# Sécurité — BOM Studio

Projet étudiant (EPITA), non déployé en production. Les données sont synthétiques (`make seed`).

## Signaler une faille

Pas d'issue publique : ouvrir un « Security advisory » privé sur le dépôt GitHub, ou prévenir
directement un membre de l'équipe. Décrire le point d'entrée, le chemin, l'effet, et comment reproduire.

## Ce qui protège le dépôt (détail : `docs/harnais.md`)

| Risque | Capteur | Où |
|---|---|---|
| secret commité | gitleaks (index au commit, commits de la PR, historique chaque nuit) | hook, CI, nightly |
| dépendance vulnérable | govulncheck (code atteint), osv-scanner (Go + npm), npm audit | CI |
| faille dans le code Go | gosec via golangci-lint | CI, `make lint` |
| workflow piégé | actions épinglées par SHA, permissions minimales, actionlint | CI |
| entrée hostile sur les montants | fuzzing `FuzzParse` | nightly |
| agent qui déborde | `.claude/settings.json` (refus), hooks `garde-fichiers`, `garde-commandes` | poste |
| inventaire | SBOM CycloneDX (Go et web) | artefact CI |

Audit complet à la demande : skill `audit-securite` (constats vérifiés par un agent distinct).
Une exception de sécurité est une décision humaine, écrite et datée (skill `trier-constat`).
