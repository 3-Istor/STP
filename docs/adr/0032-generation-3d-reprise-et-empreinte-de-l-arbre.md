# 0032 — Génération 3D : état par BOM, reprise par pièce, « stale » par empreinte de l'arbre
Statut : Proposé — Date : 2026-11-15 — Décideurs : à compléter

## Contexte
L'ADR 0031 décide une génération asynchrone « avec état persisté par pièce, reprise = relance en sautant
les pièces déjà résolues » et demande qu'une BOM modifiée après génération soit signalée (cahier des
charges, tranche 2). Elle ne fixe ni ce qui est persisté par BOM, ni comment on détecte « modifiée » :
`bom_nodes` n'a pas de colonne `updated_at`, et la scène ne dépend que de la structure de l'arbre
(nœuds, parents, pièces, quantités), pas des offres, stratégies ou jours de production.

## Options
1. **Une ligne `bom_assemblies` par BOM, écrasée à chaque relance ; `part_models` à une ligne par pièce,
   partagée par toutes les BOM ; « stale » = SHA-256 de la liste des nœuds triée par id (id, parent, pièce,
   quantité) différent de celui enregistré au lancement.** Aucun changement de schéma hors viz3d, calcul
   pur et reproductible, une relance ne refait que les pièces nouvelles. Inconvénients : pas d'historique
   des générations ; une pièce résolue pour la BOM A l'est aussi pour la BOM B (c'est voulu : le modèle
   décrit la pièce, pas son usage) ; un changement d'offre n'invalide pas la scène (il ne la change pas).
2. Historique : une ligne par lancement (`bom_assembly_runs`) — permet d'auditer, mais la reprise doit
   retrouver « le dernier lancement en cours » et l'API devrait exposer une liste ; rien ne le demande.
3. Horodatage : ajouter `updated_at` à `boms` et le toucher à chaque modification de l'arbre — change une
   table partagée entre domaines (ADR à part, AGENTS §7) et dépend d'un trigger ou de chaque chemin
   d'écriture du domaine bom ; une migration qui touche l'arbre sans passer par là rendrait la scène
   faussement « à jour ».

## Décision
Option 1 : `internal/viz3d.Fingerprint` (fonction pure), colonne `bom_assemblies.tree_fingerprint`,
statut `stale` calculé à la lecture (jamais stocké), `ClaimGeneration` en une seule instruction SQL
(`INSERT … ON CONFLICT … WHERE status NOT IN ('queued','running')`) : le refus d'une relance concurrente
ne dépend pas d'un verrou en mémoire. Au démarrage, les lignes `queued`/`running` sont relancées.

## Conséquences
- Facile : reprise après un arrêt ou une panne de base sans rien refaire ; même arbre = même empreinte ;
  un GLB téléversé pour une pièce vaut pour toutes les BOM qui l'utilisent.
- Difficile : pas d'historique ; si un jour la scène dépend d'un attribut hors de l'empreinte (offre,
  dimensions au catalogue…), il faut l'y ajouter, sinon « stale » ment par omission — un test le protège.
- Retour arrière : la colonne et la fonction disparaissent avec l'option 2 ou 3 ; `part_models` reste.
  Pivot multi-usine ou snapshot de BOM : un snapshot a sa propre liste de nœuds, donc sa propre empreinte.
