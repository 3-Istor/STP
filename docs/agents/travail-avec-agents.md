# Travailler avec des agents IA (AGENTS §6 à §16)

Texte d'origine d'`AGENTS.md`, déplacé tel quel quand `AGENTS.md` est devenu une carte (ADR 0040).
Les renvois « AGENTS §N » du code et des ADR pointent ici. Modifier ce fichier = niveau de risque 3.

## 6. L'humain décide, l'agent propose

- L'agent **cadre, propose, implémente et vérifie** ; l'humain **choisit les compromis et assume**.
- Toute décision d'architecture (§7) est **soumise à l'humain avant d'écrire du code** :
  l'agent s'arrête, présente les options, et attend une réponse explicite.
- Spec ambiguë ou cas non couvert : l'agent **pose la question** plutôt que d'inventer une règle,
  en citant la ligne de `docs/features.md` concernée.
- L'agent ne tranche jamais un désaccord dans l'équipe : il expose les arguments de chaque côté.

## 7. Qu'est-ce qu'une décision d'architecture ?

Tout ce qui est **coûteux à défaire** :

- ajouter une dépendance (Go ou npm) ou un service (cache, file de messages…) ;
- créer, fusionner ou redécouper un domaine ou une frontière (domaine / adapters) ;
- changer le schéma d'une table partagée entre domaines, ou le modèle des snapshots / cotations ;
- changer un contrat d'API `/api/v1` de façon non rétrocompatible ;
- choisir une stratégie transverse : cache, synchronisation / hors-ligne, temps réel (SSE, WebSocket),
  pannes de l'API de prix, feature toggles, versionnement des facteurs carbone ;
- toute règle de calcul absente de la spec (arrondi, ordre d'application des remises…).

Ne l'est pas : un nom de variable, un refactor local, un test en plus, un composant UI isolé.

## 8. ADR (Architecture Decision Records)

- Emplacement : `docs/adr/NNNN-titre-en-kebab.md` (numérotation continue, `0001`, `0002`…).
- L'agent **rédige le brouillon** au statut `Proposé` ; **seul un humain** le passe à `Accepté`
  et inscrit son nom dans `Décideurs`. **Un seul membre de l'équipe suffit** : pas besoin
  de l'accord des trois.
- Une ADR acceptée **ne se réécrit pas** : on en crée une nouvelle qui la remplace (`Remplacée par 00XX`).
- Une page max. Format :

```markdown
# NNNN — Titre de la décision
Statut : Proposé | Accepté | Remplacé par NNNN — Date : AAAA-MM-JJ — Décideurs : <noms>

## Contexte
Le besoin, la contrainte ou le risque (lien vers la feature / l'invariant).

## Options
1. Option A — avantages / inconvénients
2. Option B — avantages / inconvénients
(au moins deux, dont « ne rien faire » si pertinent)

## Décision
L'option retenue, en une phrase, et pourquoi elle gagne **dans ce contexte**.

## Conséquences
Ce que ça rend facile, ce que ça rend difficile, comment on reviendrait en arrière.
Impact si le pivot arrive (hors-ligne, multi-source de prix, carbone versionné, multi-usine).
```

- Le code qui implémente une ADR la référence (message de commit ou commentaire : `voir ADR 0003`).

## 9. Pas de code orphelin

*Si l'équipe ne peut pas l'expliquer, ce code ne lui appartient pas.* À chaque livraison, l'agent fournit :

1. **ce que fait le changement** et la responsabilité de chaque fichier touché ;
2. **un cas de panne prévisible** (entrée, état ou dépendance qui le ferait échouer) ;
3. **le test qui le met en défaut** (ou qui prouve qu'il résiste à ce cas) ;
4. **ce qui n'est pas fait ou pas vérifié**, dit honnêtement.

Du code simple qu'un humain modifie sans nouveau prompt vaut mieux que du code « malin ».

## 10. Petits pas vérifiables (Test & Learn)

- Hypothèse → prototype → test → mesure → décision. Une erreur doit être **rapide, observable, réversible**.
- Livrer des **tranches verticales** (de la BOM au résultat expliqué) plutôt qu'une couche complète sans usage.
- Un commit = une intention. Pas de commit géant mélangeant refactor, feature et formatage.
- Avant de modifier un comportement existant : **un test qui protège l'existant d'abord**, puis le changement.

## 11. Ce que l'agent ne fait jamais sans accord explicite

- pousser, merger, ouvrir une PR, réécrire l'historique git ;
- ajouter une dépendance ;
- modifier une migration déjà commitée, une ADR acceptée, ou `docs/features.md` ;
- désactiver, supprimer ou affaiblir un test (ou un job de CI) pour faire passer la pipeline ;
- toucher aux données d'un snapshot publié (même en base de dev).

## 12. Préparer le pivot sans l'anticiper

- Le pivot (hors-ligne strict, multiples sources de prix, carbone versionné, multi-usine / multi-devise)
  sera imposé après la soutenance 1. On **ne code pas** ces cas « au cas où », mais on **ne ferme pas
  la porte** : frontières nettes, pas de dépendance à l'heure ou au réseau dans le domaine, données versionnées.
- Au moment du pivot : **cartographier l'impact avant de coder** (fichiers, ADR concernées, tests à ajouter),
  proposer à l'humain, migrer progressivement.

## 13. Équipe multi-IA

- **Mêmes règles quel que soit l'outil** : ce fichier fait foi. Un fichier propre à un outil contient
  seulement un renvoi vers `AGENTS.md` (ex. `CLAUDE.md` = `@AGENTS.md`).
  Codex, Copilot et Cursor lisent `AGENTS.md` directement ; pour Gemini CLI, créer `GEMINI.md` avec `@AGENTS.md`.
- Préférences personnelles (modèle, style, raccourcis) : fichiers locaux non commités
  (`CLAUDE.local.md`, réglages de l'éditeur…), jamais dans les règles partagées.
- **Traçabilité** : un commit écrit avec l'aide d'une IA le dit dans un trailer
  (`Co-Authored-By: <outil / modèle> <adresse>`). L'humain qui commite reste l'auteur et le responsable.
- Une IA peut relire le code d'une autre (ou d'un humain) ; la relecture finale avant merge est **humaine**.
- Un agent ne reprend pas en cours la branche d'un autre sans que l'humain propriétaire soit d'accord.

## 14. Ownership partagé

- L'agent écrit pour **toute l'équipe**, pas pour la personne qui l'utilise : noms explicites,
  ADR et commits qui racontent le pourquoi.
- Le dépôt doit raconter la même histoire que la soutenance : chaque choix important se retrouve
  dans une ADR, chaque règle métier dans un test.

## 15. Définition de « terminé »

Une tâche n'est terminée que si :

- `make lint` et `make test` passent en local, et la CI est verte ;
- le livrable du §9 est fourni ;
- une ADR existe si une décision du §7 a été prise.

Sinon l'agent le dit, avec la sortie de la commande qui échoue.

## 16. Pipeline CI

`.github/workflows/ci.yml`, sur chaque push vers `main` et chaque PR (voir ADR 0001, `docs/workflow.md`) :

- **go** (service Postgres 18) : `go mod tidy -diff`, code sqlc à jour, migrations `goose up` / `reset` / `up`,
  `go vet`, `golangci-lint` (dont `gosec`), `go test -race` + couverture (non bloquante), tests d'intégration compris ;
- **web** : `npm ci`, `oxlint`, build (`tsc` + Vite), `npm audit` ;
- **security** : `govulncheck`.

Une PR ne se merge pas si la CI est rouge. On ajoute une étape quand le besoin existe
(ex. service Postgres + `goose up` dès le premier test qui touche la base).
