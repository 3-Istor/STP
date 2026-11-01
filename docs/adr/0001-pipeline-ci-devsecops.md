# 0001 — Pipeline CI : qualité, sécurité et couverture
Statut : Proposé — Date : 2026-11-01 — Décideurs : à compléter

## Contexte
La CI actuelle rejoue lint, tests et build (AGENTS §16). Il manque : un contrôle des failles connues dans les
dépendances, une analyse de sécurité du code, une mesure de couverture.
Le workflow d'équipe (`docs/workflow.md`) s'appuie sur ces contrôles.
Ajouter un outil est une décision §7 (dépendance). Contrainte : dépôt privé sur le plan gratuit de
l'organisation, donc pas de *secret scanning*, de *push protection*, de `dependency-review-action`
ni de check requis sur `main` (fonctions payantes sur dépôt privé).

## Options
1. **Ne rien faire** — zéro coût ; aucune visibilité sur les failles, couverture invisible.
2. **Contrôles natifs / officiels uniquement** — `gosec` (déjà embarqué par golangci-lint), `govulncheck`
   (outil de l'équipe Go, ajouté en `tool` dans `go.mod`), `npm audit` (inclus dans npm), Dependabot
   (gratuit), couverture via `go test -coverprofile` (stdlib).
   Pas de service externe, pas de secret. Inconvénient : pas de SAST côté TypeScript.
3. **Option 2 + CodeQL + gitleaks** — SAST Go et TS, scan de secrets dans l'historique. Inconvénients :
   CodeQL n'est gratuit que sur dépôt public ; `gitleaks-action` demande une licence pour une organisation ;
   temps de CI ×2.
4. **Seuil de couverture bloquant (ex. 80 %)** — en plus de 2 ou 3. Inconvénient : contredit
   « pas de couverture décorative » (AGENTS §4).

## Décision
Option 2 : elle couvre les risques réels d'un projet sans authentification ni secret de prod, avec des
outils officiels, gratuits, rejouables en local, sans compte externe. Couverture affichée mais non bloquante.
Secrets : pas de scan automatique ; `.env` ignoré par git et relecture du diff en revue.

## Conséquences
- Facile : une faille dans une dépendance rend la CI rouge ; `make sec` rejoue la même chose en local.
- Difficile : rien n'empêche techniquement de merger une PR rouge ; c'est une règle d'équipe (AGENTS §16).
- Difficile : `govulncheck` peut bloquer une PR sans rapport quand une faille sort — on met à jour la
  dépendance dans une PR dédiée, ou on accepte explicitement l'alerte (raison + date de réexamen).
- Retour arrière : supprimer le job et la ligne `tool` de `go.mod`.
- Pivot : multi-sources de prix → les nouveaux clients d'API passent par `gosec`/`govulncheck` sans changement ;
  IA (P3) → workflow d'évaluation manuel séparé (voir `docs/workflow.md` §6), à décider dans une ADR dédiée.
- À reconsidérer si le dépôt devient public ou passe au plan Team : secret scanning, push protection,
  dependency review, check requis sur `main`, CodeQL.
