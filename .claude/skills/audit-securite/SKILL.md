---
name: audit-securite
description: Audit de sécurité complet de BOM Studio (API Go, PostgreSQL, front React, CI), en phases, avec des constats vérifiés par un agent distinct de celui qui les a trouvés. À utiliser quand on demande un audit, une revue de sécurité ou de chercher des failles, pas pour la relecture d'un simple diff.
---

# Audit de sécurité (méthode inspirée de Cloudflare)

Méthode adaptée de `cloudflare/security-audit-skill` (MIT) : séparer **trouver** et **confirmer**,
n'écrire que des failles exploitables, sévérité = probabilité × impact. Pour l'outil complet de
Cloudflare : `npx skills add https://github.com/cloudflare/security-audit-skill --skill security-audit`
(dépendance externe : accord humain requis, ADR 0040).

Sortie : `.audit/<date>/` (non commité) — architecture.md, findings.json, REPORT.md.

## Phase 1 — Reconnaissance

Cartographier sans juger : points d'entrée (`internal/httpapi`, flux SSE `/api/v1/quotes/stream`,
`cmd/priceapi`, `cmd/server seed`), frontières de confiance (navigateur → nginx → API → PostgreSQL ;
API → source de prix externe), données sensibles (prix fournisseurs, snapshots publiés), et ce que les
capteurs couvrent déjà (gosec, govulncheck, gitleaks, osv-scanner, `internal/archtest`).
Écrire `architecture.md` et une liste d'unités à couvrir (une route ou un adapter = une unité).

## Phase 2 — Chasse, unité par unité

Un sous-agent par groupe d'unités, chacun avec seulement son périmètre. Pistes propres au projet :
- entrées non bornées : corps JSON, profondeur d'arbre BOM, listes d'IDs, exposant d'un montant ;
- injection SQL hors sqlc, tri ou filtre dynamique ;
- source de prix : réponse énorme, lente, aberrante, devise inconnue (l'adapter doit borner et rejeter) ;
- SSE : connexions illimitées, fuite de goroutines, données d'un autre contexte ;
- immuabilité : un chemin qui modifie un snapshot publié malgré les triggers ;
- front : HTML injecté (`dangerouslySetInnerHTML`), URL construite depuis une donnée ;
- CI : `pull_request_target`, expressions `${{ }}` dans `run:`, actions non épinglées, permissions larges.

Chaque candidat : point d'entrée → chemin (fichier:ligne) → effet observable.

## Phase 3 — Validation par un agent neuf

Chaque candidat va à un **autre** sous-agent qui essaie de le **réfuter** (garde existant, validation en
amont, chemin impossible). Verdicts : `confirmed` (trace complète + effet borné observé, idéalement un
test ou une requête `curl` sur `make up`), `needs_validation` (le fait manquant exact, sans sévérité),
`rejected` (la raison).

## Phase 4 — Enregistrer

`findings.json` : un objet par candidat — `id`, `verdict`, `title`, `entry_point`, `trace` (liste de
`fichier:ligne`), `impact`, `likelihood`, `severity` (critique/élevée/moyenne/faible, absente si
`needs_validation`), `evidence`, `fix`. Vérifier que le JSON se lit (`jq . findings.json`).

## Phase 5 — Revérifier les preuves

Un agent neuf relit chaque `confirmed` contre le code **actuel** (lignes, noms) ; une correction de fond
repasse en phase 3.

## Phase 6 — Rapport

REPORT.md : résumé en 5 lignes, tableau des `confirmed` par sévérité, `needs_validation` à part,
couverture (unités vues / non vues). Chaque constat confirmé devient un ticket ; la correction suit
`trier-constat`. Aucune faille ni preuve d'exploitation n'est publiée hors du dépôt.
