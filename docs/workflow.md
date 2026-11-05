# Workflow de l'équipe

Comment une feature passe de `docs/features.md` à `main`. Les règles de fond sont dans
[AGENTS.md](../AGENTS.md) ; ce document décrit le **chemin** et les **portes** à franchir.

```
features.md ──► Issue (DoR) ──► ADR ? ──► Branche ──► PR (CI + revue humaine) ──► main (DoD)
```

---

## 1. Definition of Ready (DoR)

Une feature (ou une tranche de feature) **n'entre pas en développement** tant que l'issue
ne coche pas toutes ces cases. Le modèle d'issue `.github/ISSUE_TEMPLATE/feature.md` les reprend.

- [ ] **Source** : lien vers la section de `docs/features.md` (nom de la feature + exigences Must couvertes).
- [ ] **Dépendances** : les features listées dans « Dépend de » sont terminées, ou la tranche s'en passe.
- [ ] **Tranche verticale** : la tranche va de la donnée au résultat visible/expliqué (AGENTS §10),
      livrable en une PR de taille raisonnable (≈ 400 lignes hors code généré).
- [ ] **Critères d'acceptation** : recopiés de la spec au format QUAND/SI … ALORS … DOIT (voir §2),
      chacun vérifiable par un test automatisé, ou explicitement marqué « vérification manuelle ».
- [ ] **Ambiguïtés levées** : toute question sur la spec a reçu une réponse humaine (AGENTS §6) ;
      aucune règle de calcul inventée (arrondi, ordre des remises…).
- [ ] **Décisions d'architecture identifiées** : si la tranche touche à un point de AGENTS §7
      (dépendance, schéma partagé, contrat d'API, stratégie transverse), l'ADR est au moins `Proposé`.
- [ ] **Données de test** : on sait avec quelles données on prouvera les critères (fixtures, graine du simulateur).
- [ ] **Responsable** : un humain porteur nommé (l'agent IA n'est jamais propriétaire).

## 2. Critères d'acceptation

Format unique, celui de la spec (EARS) :

| Forme | Usage |
|---|---|
| `QUAND <événement> ALORS <le système> DOIT <résultat>` | comportement nominal |
| `SI <condition indésirable> ALORS <le système> DOIT <refuser / signaler>` | cas d'erreur, invariant |
| `… NE DOIT PAS …` | interdiction (pas de prix à zéro, pas de doublon…) |

Règles :

- **Un critère = au moins un test** qui échoue si le critère est violé. Le nom du sous-test cite le critère :
  ```go
  {name: "SI snapshot publié ALORS modification refusée", ...}
  ```
- Les critères de la spec ne se modifient pas dans le code ou la PR (AGENTS §11) : un critère manquant ou
  faux se remonte à l'humain, qui met à jour `docs/features.md` dans un commit à part.
- Un critère non automatisable (UI, perf) est listé dans la PR avec la façon dont il a été vérifié.

## 3. Le chemin d'une feature

1. **Issue** créée depuis le modèle « Feature », DoR cochée → étiquette `ready`.
2. **ADR** si besoin (AGENTS §8) : l'agent rédige au statut `Proposé`, un humain accepte.
   Pas de code qui dépend d'une ADR non acceptée.
3. **Branche** depuis `main` : `feat/<feature>-<tranche>`, `fix/<sujet>`, `docs/<sujet>`, `ci/<sujet>`.
4. **Commits** : un commit = une intention (AGENTS §10). Trailer `Co-Authored-By:` si une IA a aidé (§13).
   Avant de modifier un comportement existant : un commit qui ajoute le test protecteur, puis le changement.
5. **PR** vers `main` avec le modèle `.github/pull_request_template.md` (livrable AGENTS §9 + critères cochés).
6. **CI verte** (voir §7) — tous les jobs. Le plan gratuit de l'organisation n'impose pas de check requis
   sur un dépôt privé : la règle « pas de merge si la CI est rouge » est une règle d'équipe (AGENTS §16).
7. **Revue humaine obligatoire** (1 approbation minimum), même si une IA a déjà relu.
8. **Merge** en *squash* par un humain ; la branche est supprimée ; l'issue est fermée par la PR (`Closes #…`).

### Rôles

| Qui | Fait | Ne fait pas |
|---|---|---|
| Humain porteur | DoR, choix des compromis, acceptation des ADR, merge | déléguer la responsabilité au prompt |
| Agent IA | cadrage, brouillon d'ADR, code, tests, livrable §9 | pousser, merger, ajouter une dépendance, toucher une ADR acceptée |
| Relecteur humain | lit le diff **et** le livrable §9, rejoue un critère au besoin | approuver un code qu'il ne sait pas expliquer |

## 4. Definition of Done (DoD)

Reprend AGENTS §15, vérifiée dans la PR :

- [ ] Tous les critères d'acceptation de la tranche ont leur test, et ils passent.
- [ ] `make lint` et `make test` passent en local ; la CI est verte.
- [ ] Livrable §9 dans la description de PR (quoi, cas de panne, test qui le prouve, non fait / non vérifié).
- [ ] ADR acceptée si une décision §7 a été prise, référencée dans le commit.
- [ ] Aucune nouvelle alerte de sécurité non traitée (voir §6).
- [ ] Revue humaine approuvée.

## 5. Couverture de test

La couverture est **un indicateur, pas un objectif** (AGENTS §4 : pas de couverture décorative).

- La CI calcule la couverture (`go test -race -coverprofile`) et l'affiche dans le résumé du job ;
  le fichier `coverage.out` est publié en artefact.
- **Pas de seuil global bloquant** : un seuil pousse à écrire des tests qui ne peuvent pas échouer.
- Ce qui compte et se vérifie en revue :
  - chaque critère d'acceptation a son test (§2) ;
  - chaque calcul métier (monnaie, arbre, remises, budgets) a des tests table-driven, cas limites compris ;
  - chaque erreur sentinelle est testée avec `errors.Is`.
- La revue regarde la couverture **des packages de domaine** (`internal/<domaine>`) : une baisse sur un
  package de calcul doit être justifiée dans la PR.
- Plus tard (quand le domaine existera) : seuil minimal par package de calcul uniquement, à décider en ADR.

## 6. DevSecMLOps

### Dev — qualité
`go vet`, `golangci-lint` (dont `gosec`), `go test -race`, `oxlint`, `tsc` + build Vite, à chaque push et PR.

### Sec — sécurité
| Risque | Contrôle | Où |
|---|---|---|
| Faille connue dans une dépendance Go | `govulncheck` (n'alerte que si le code appelle la fonction vulnérable) | CI, job `security` |
| Faille connue dans une dépendance npm | `npm audit --omit=dev --audit-level=high` | CI, job `web` |
| Code dangereux (injection SQL, chemins, crypto faible…) | `gosec` via golangci-lint | CI, job `go` |
| Secret commité | `.env` ignoré par git, relecture du diff en revue (pas de scan automatique : payant sur dépôt privé) | `.gitignore`, revue |
| Pipeline détournée | `permissions: contents: read`, actions officielles uniquement, pas de secret exposé aux PR de forks | `ci.yml` |
| Dépendances qui vieillissent | Dependabot (go, npm, github-actions), PR hebdo revues par un humain | `.github/dependabot.yml` |

Une alerte se **corrige** ou s'**accepte** explicitement (commentaire dans la PR avec la raison et une date
de réexamen) ; elle ne se masque jamais en désactivant le contrôle (AGENTS §11).

### MLOps — l'IA dans le produit (features P3, rien à coder avant)
Préparé sans être anticipé (AGENTS §12) ; à activer quand l'interface IA existera :

- **Prompts versionnés** dans le dépôt, comme du code ; chaque résultat IA indique le modèle et la version du
  prompt utilisés (exigence « résultats explicables »).
- **Faux fournisseur déterministe** implémentant l'interface Go : les tests et la CI n'appellent jamais un
  vrai LLM (reproductibles, gratuits, sans secret).
- **Jeu d'évaluation** (questions → réponses/modifications attendues sur une BOM de référence) rejoué contre
  le vrai fournisseur dans un workflow **manuel** (`workflow_dispatch`), jamais sur une PR.
- **Garde-fou testé** : une proposition IA ne s'applique qu'à un brouillon, après confirmation ;
  un test prouve qu'elle est refusée sur un snapshot.
- Changer de modèle ou de prompt = rejouer l'évaluation et noter le résultat dans la PR.

## 7. Pipeline CI

Fichier `.github/workflows/ci.yml`, déclenché sur push vers `main` et sur chaque PR. Décision : ADR 0001.

```
          ┌── go ──────────── tidy · sqlc à jour · goose up/reset/up · vet · golangci-lint (+gosec)
          │                   · test -race + couverture (service Postgres 18)
push/PR ──┼── web ─────────── npm ci · oxlint · tsc+build · Playwright (3 moteurs) · npm audit
          ├── web-live ────── source HTTP · ingestion · Postgres 18 · SSE · Playwright réel (3 moteurs)
          ├── compose ─────── images · services sains · migrations · santé via nginx
          └── security ────── govulncheck
```

Rejouer en local : `make migrate lint test cover sec`, puis `make sqlc && git status` (aucun changement attendu).
Les tests d'intégration lisent `DATABASE_URL` (exportée par le `Makefile`) : sans base joignable, ils sont
sautés en local mais **échouent en CI** (`CI=true`).

Tests UI (voir ADR 0013) : dans `web`, `npm ci`, installation des navigateurs Playwright,
puis `npm run test:e2e`. Les parcours sont rejoués sur Chromium, Firefox et WebKit, en formats
ordinateur et mobile. Ils contrôlent couleurs calculées, précision/arrondi monétaire, navigation,
graphiques et relecture REST sur signal contrôlé. Un test mocké ne constitue pas une vérification
du simulateur/SSE réel ; les contrôles Safari réel restent tracés séparément dans la spec.

Le job `web-live` ajoute les douze parcours réels de NLPF-68 sans remplacer les parcours contrôlés.
Il lance `NLPF68_BROWSER_MODE=native go test -race -count=1 -timeout=4m -run
'^TestPricingLiveBrowsers$' ./internal/pricing/pricingstore`, avec une base jetable et les navigateurs
installés. Le test crée un schéma privé, une vraie source HTTP et un worker à horloge pilotée ;
le navigateur utilise EventSource et le proxy Vite réels. Échec, capture et trace sont conservés
comme artefacts. Safari macOS/iOS et les raccordements de la vraie BOM restent des preuves séparées.

Mesure de prix bornée, opt-in : `NLPF68_PRICING_MEASURE=true go test -v -count=1 -timeout=120s
-run '^TestPricingCatalogueMeasurement$' ./internal/pricing/pricingstore`. Elle distingue 30 000
nœuds de 300 offres uniques et ne mesure pas le recalcul du moteur BOM absent.
