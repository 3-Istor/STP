---
name: rediger-adr
description: Rédiger une ADR (Architecture Decision Record) au statut Proposé pour BOM Studio. À utiliser avant d'ajouter une dépendance, de changer un schéma partagé, un contrat /api/v1 non compatible, une stratégie transverse ou une règle de calcul absente de la spec.
---

# Rédiger une ADR

L'agent rédige, **l'humain décide**. Une ADR n'est jamais passée à `Accepté` par un agent.

## Quand

Tout ce qui est coûteux à défaire (`docs/agents/travail-avec-agents.md` §7) : dépendance ou service,
domaine ou frontière, schéma partagé ou modèle des snapshots, contrat `/api/v1` non compatible,
stratégie transverse (cache, hors-ligne, temps réel, pannes de l'API de prix, toggles, carbone versionné),
règle de calcul absente de la spec. Pas pour un nom, un refactor local, un test.

## Étapes

1. Numéro suivant : `ls docs/adr | tail -1`. Fichier `docs/adr/NNNN-titre-en-kebab.md`.
2. Une page au plus, ce gabarit exact :

```markdown
# NNNN — Titre de la décision
Statut : Proposé — Date : AAAA-MM-JJ — Décideurs : (à compléter par un humain)

## Contexte
Le besoin, la contrainte ou le risque (lien vers la feature ou l'invariant).

## Options
1. Option A — avantages / inconvénients
2. Option B — avantages / inconvénients
(au moins deux, dont « ne rien faire » si pertinent)

## Décision
L'option recommandée, en une phrase, et pourquoi elle gagne dans ce contexte.

## Conséquences
Ce que ça rend facile, difficile, comment revenir en arrière.
Impact si le pivot arrive (hors-ligne, multi-source de prix, carbone versionné, multi-usine).
```

3. Mettre **en tête de la Décision** ce que l'humain doit trancher, avec la recommandation et le coût d'un non.
4. Arrêter le travail qui en dépend et présenter l'ADR à l'humain. Continuer seulement ce qui n'en dépend pas.
5. Une fois acceptée par un humain : le code qui l'implémente la cite (`voir ADR NNNN`).

## Interdits

- Réécrire une ADR acceptée : on en crée une nouvelle et on marque l'ancienne `Remplacée par NNNN`.
- Écrire une seule option, ou des options fictives pour la forme.
- Remplir `Décideurs` à la place d'un humain.
