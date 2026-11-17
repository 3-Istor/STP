---
name: revue-par-niveau
description: Relire un changement de BOM Studio avant de le déclarer terminé, avec les relecteurs IA qu'exige son niveau de risque (0–3) et au plus trois tours. À utiliser à la fin de toute tâche de code, avant un commit final ou une PR.
---

# Revue par niveau de risque

Le niveau dit **qui** relit ; `REVIEW.md` dit **comment** noter et **ce qui remonte** à l'humain.

## 1. Moulinette d'abord

```sh
make verify LEVEL=pr
```

Rouge → on corrige avant toute relecture IA : un relecteur ne rend pas vert ce que la moulinette refuse.
Une étape `not_run` est dite comme telle dans le livrable, jamais comptée verte.

## 2. Niveau et relecteurs

```sh
go run ./cmd/risk            # base : merge-base avec origin/main
```

| Niveau | Relecteurs (sous-agents `.claude/agents/`, lancés en parallèle) |
|---|---|
| 0 | aucun |
| 1 | `relecteur-code` |
| 2 | `relecteur-code`, `relecteur-architecture` |
| 3 | les quatre : + `relecteur-securite`, `relecteur-metier` |

Chaque relecteur reçoit **seulement** : la base git, le niveau et ses raisons, la liste des fichiers.
Pas la conversation, pas le plan, pas « j'ai déjà vérifié X » : le contexte neuf est tout l'intérêt.

## 3. Traiter les constats (skill `trier-constat` pour chacun)

- `Important` vérifié ou probable → corriger, avec un test qui échouait avant.
- Classe de bug → corriger **toutes** les occurrences, pas seulement la ligne citée.
- `Important` spéculatif → chercher la preuve une fois ; sinon l'écarter en le disant.
- `Nit` → corriger si c'est trivial, sinon une ligne dans le résumé.
- `Pré-existant` → ticket ou « Questions ouvertes » de `state.md`, hors de ce diff.

## 4. Tours suivants

- Relancer seulement les relecteurs qui avaient un `Important`, sur le **nouveau diff** uniquement.
- Ne pas leur redonner les constats déjà traités.
- **Au plus 3 tours.** Encore un `Important` au 3ᵉ : s'arrêter, dire à l'humain ce qui résiste et
  proposer de revoir l'approche plutôt que d'empiler des correctifs.

## 5. Résumé pour l'humain

Dans la PR ou la réponse finale, une section courte :

```
Niveau 3 (internal/money/exact.go : montants). Moulinette pr : verte (intégration : not_run en local).
Relecture : 2 tours. Corrigés : 1 Important métier (arrondi intermédiaire, TestRoundOnce).
Écartés : 1 spéculatif sécurité (pas de chemin d'entrée). Nits : 2 (noms). Pré-existant : 1 → nlpf-74.
À relire par vous : exact.go:60-110 (arrondi), migrations/00016 (trigger).
```

La dernière ligne est la plus importante : **où** l'humain doit regarder. L'approbation d'un relecteur IA
ne remplace jamais celle d'un humain (niveau 1 : lit ce résumé ; 2 : approuve ; 3 : deux humains).
