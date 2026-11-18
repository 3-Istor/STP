# 0049 — Liaison explicite des assets aux occurrences BOM
Statut : Proposé — Date : 2026-11-17 — Décideurs : (à compléter par un humain)

Renumérotation lors de l’intégration de `main` : ancien numéro 0041 sur `feat/3d-pro-v2`, déjà utilisé pour une autre décision sur `main`. Le contenu de la décision reste inchangé.

## Contexte
La mission de refonte 3D demande des pièces sélectionnables, remplaçables et répétées sans confusion
avec une offre fournisseur. L'ADR 0035 couvre des schémas de sous-ensembles associés par leur nom ;
elle ne définit pas les liaisons d'une géométrie réelle aux occurrences. Aucun des 418 types n'a de
géométrie constructeur ni de placement vérifiés acquis. Voir `docs/3d/AUDIT.md` et `3D_PLAN.md`.

## Options
1. **Manifestes frontend explicites liés à une installation** : réutiliser Three.js et les IDs de
   l'API existante ; fichier statique local versionné, avec registre d'assets et liaisons d'occurrences.
   Aucun schéma ni contrat API modifié ; association à rétablir explicitement sur une autre base.
2. **Identité d'asset persistée dans le catalogue et exposée par l'API** : portable entre manifestes,
   mais schéma, services, API et comportement des versions publiées à définir et à faire approuver.
3. **Conserver uniquement les schémas par sous-ensemble** : faible coût, mais la sélection d'une
   vraie pièce et les remplacements géométriques de la mission restent impossibles.

## Décision
**À trancher : autoriser l'option 1 pour un premier rendu lié aux pièces.** Elle est recommandée pour
avancer sans migration, dépendance ni changement des calculs ; la refuser reporte la liaison fine.
Le contrat proposé distingue :
- une identité de scène explicite (`draft` ou `snapshot`, ID), une révision et une provenance ;
- un registre d'assets : clé logique, chemin local, empreinte, licence, auteur et preuve de géométrie ;
- des liaisons : `node_id` + `part_id` attendus, clé d'asset, chemin du mesh, placement et pivots
  documentés, instances et déplacements d'éclatement. Aucune liaison par nom BOM ou `offer_id`.

Le frontend valide la portée et l'identité contre l'arbre chargé. ID inconnu, pièce remplacée sans
liaison compatible, mesh absent ou emplacement inconnu → état indisponible, ancien mesh retiré.
Une quantité n'ajoute d'instances qu'aux emplacements explicitement fournis. `offer_id` ne choisit
jamais une géométrie. Géométrie et placement ont des preuves séparées ; un schéma reste un schéma.
Pour un snapshot, un manifeste doit être explicitement lié à cette version et à une révision figée ;
sinon, aucune liaison fine n'est annoncée. Aucune donnée de snapshot publiée n'est modifiée.

## Conséquences
Le chargeur, le picking, la caméra et les transitions existants sont réutilisables ; une même pièce
peut être liée à plusieurs occurrences distinctes. Un manifeste importé d'une autre base doit être
réassocié et validé ; les IDs du seed ne sont jamais supposés universels. Les modèles fidèles et
leurs droits restent à obtenir. Les schémas de l'ADR 0035 sont conservés avec leur statut explicite.
Retour arrière : retirer le rendu lié au manifeste, sans migration de données. Hors-ligne : fichiers
locaux possibles ; multi-usine : portées distinctes à réexaminer ; prix et carbone restent hors du
viewer. Une intégration persistée ultérieure ferait l'objet d'une nouvelle décision.
