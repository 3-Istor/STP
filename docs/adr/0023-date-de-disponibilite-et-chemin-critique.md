# 0023 — Date de disponibilité, date cible et chemin critique
Statut : Accepté — Date : 2026-11-14 — Décideurs : Arthur.presle (accord explicite dans le chat)

## Contexte
lead-time-planning (`docs/features.md:321–340`) : date de disponibilité calculée à partir des délais,
date cible saisie, pièces du chemin critique et pièces responsables si la cible est impossible.
Une **date** suppose une date de départ ; or le domaine ne lit jamais l'heure courante (AGENTS §3).
Le délai total par nœud existe déjà (ADR 0007, 0008 : max des enfants + délai propre, jours calendaires).

## Options
1. Date de départ = `now()` côté serveur — simple ; viole la reproductibilité (même snapshot, autre jour,
   autre résultat).
2. Date de départ et date cible **paramètres de la BOM** (table `bom_schedule (bom_id, start_on DATE,
   target_on DATE NULL)`, copiée au snapshot) — reproductible et explicable ; une table de plus.
3. Date de départ passée en paramètre de requête, rien n'est stocké — pas de schéma ; la cible n'est
   pas retrouvée d'une session à l'autre ni figée à la publication.

## Décision
**option 2.** Le calcul reste pur : `disponibilité = start_on + délai racine (jours calendaires)`.

## Conséquences
- Chemin critique : les feuilles dont la somme des délais propres jusqu'à la racine égale le délai racine ;
  égalités conservées (plusieurs chemins critiques), triées par id.
- Cible impossible (`start_on + délai racine > target_on`) : liste des pièces dont le chemin dépasse
  `target_on − start_on` jours, avec le dépassement de chacune.
- Should (alternatives plus rapides) : autres offres de la pièce et alternatives du catalogue au délai
  plus court, et stock (ADR 0022) ; propositions seulement, appliquées au brouillon après confirmation.
- Sans `start_on` : délai en jours seulement, pas de date (pas de date inventée).
- Retour arrière : suppression de la table ; le délai en jours existant n'est pas touché.
- Pivot hors-ligne : aucune dépendance au réseau ni à l'horloge ; multi-usine : un calendrier par usine.
