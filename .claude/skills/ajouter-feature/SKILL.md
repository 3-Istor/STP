---
name: ajouter-feature
description: Implémenter une feature de docs/features.md en tranche verticale (BOM → résultat expliqué) dans BOM Studio. À utiliser quand on demande d'ajouter, implémenter ou livrer une feature, un écran ou un calcul nouveau.
---

# Ajouter une feature (tranche verticale)

Objectif : une feature utilisable de bout en bout, prouvée par des tests, que l'équipe sait expliquer.

## 1. Cadrer (avant tout code)

1. Lire la feature dans `docs/features.md` et recopier ses critères d'acceptation **mot pour mot**.
2. Lister ce qui manque ou est ambigu. Spec muette sur une règle de calcul (arrondi, ordre des remises…)
   = décision humaine : écrire la question avec la ligne citée, ne pas inventer.
3. `make risk` sur les fichiers prévus. Niveau ≥ 2 : présenter le plan à l'humain et **attendre**.
4. Décision coûteuse à défaire (dépendance, schéma partagé, contrat cassant, stratégie transverse) :
   skill `rediger-adr`, statut `Proposé`, on attend l'acceptation.
5. Créer `.specs/<ticket>/spec.md` (intention, critères, tâches T1…Tn) et `state.md` (avancement,
   décisions prises seul, questions ouvertes). Branche `<ticket>`, jamais `main`.

## 2. Construire, couche par couche, un test d'abord

| Couche | Où | Test |
|---|---|---|
| Règle métier | `internal/<domaine>/` | table-driven, cas nominal + limites + erreur nommée (`errors.Is`) |
| Persistance | `migrations/` + `internal/<d>/queries.sql` + `go tool sqlc generate` | skill `ajouter-migration`, test d'intégration |
| API | `internal/httpapi/` (décoder → service → encoder) | `httptest`, skill `changer-contrat-api` |
| Écran | `web/src/` | test unitaire de la logique pure + parcours Playwright si nouveau parcours |

Règles à ne jamais oublier (`docs/agents/regles.md` §3) : pas de `float` pour un montant, devise toujours
présente, arrondi seulement en sortie, pas d'horloge ni d'aléa dans le domaine, donnée manquante = erreur
nommée, résultat qui dit sur quoi il repose (version, snapshot, taux et date).

Un commit par intention : `<ticket>: T<N> - <ce qui change>` + trailer `Co-Authored-By`.

## 3. Prouver

1. Chaque critère d'acceptation → le nom du test qui le prouve (dans la PR).
2. Vérifier qu'un test nouveau **peut échouer** : casser la règle une fois, voir rouge, remettre.
3. `make verify` pendant le travail, `make verify LEVEL=pr` avant de pousser.
4. Lancer l'app (`make dev`) et parcourir l'écran : un test vert ne prouve pas que l'écran est lisible.

## 4. Relire et livrer

1. Skill `revue-par-niveau`.
2. Livrable (`docs/agents/travail-avec-agents.md` §9) dans la PR et `state.md` :
   ce que fait le changement et le rôle de chaque fichier ; un cas de panne prévisible ; le test qui le
   couvre ; ce qui n'est pas fait ou pas vérifié, dit honnêtement.
3. Mettre `docs/feature_implemented.md` à jour si la feature est terminée.

## Pièges connus dans ce dépôt

- `npm install` casse le lockfile : `npm ci`, et `npx npm@11 install <pkg>` seulement avec accord.
- Les tests d'intégration sautent sans `DATABASE_URL` : ils ne sont **pas** verts en local, la CI les lance.
- Le temps est simulé (`SIM_YEAR_DURATION`) : un contrat de démo peut expirer pendant un test manuel.
