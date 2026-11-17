# 0035 — Vue 3D par sous-ensemble, rendus Simple et Réaliste
Statut : Proposé — Date : 2026-11-17 — Décideurs : (à compléter par un humain)

## Contexte
Vision V2 §6 et maquettes I4-2 / I4-3 : la 3D sert à **localiser** (clic → nœud de BOM, sélection → mise en
évidence, coloration budget, éclaté, focus roue). L'audit métier la classe en gadget : elle ne doit ni alourdir
l'application, ni inventer de données. Prototypes jetables validés : `docs/v2/prototype-3d/`.

## Options
1. **Niveau de représentation** — a) une forme par pièce (milliers d'objets, géométrie inventée pour chaque
   pièce) ; b) **une forme par sous-ensemble** (niveau 2, ~56 sur la Multipla), pièces visibles dans l'arbre.
2. **Source des formes** — a) formes générées (aucune licence, pas réaliste) ; b) modèle sous licence relié
   par table (réaliste, mais pas le bon véhicule) ; c) les deux, même correspondance ; d) ne rien faire.
3. **Chargement** — a) `three` dans le bundle principal ; b) `React.lazy` + import dynamique.

## Décision
1b + 2c + 3b. Un **emplacement** (slot) relie un nom de sous-ensemble (`data/slots.json`, regex sur le nom
normalisé) à une forme Simple (`data/simple-layout.json`) et à des nœuds du modèle réaliste
(`data/realistic-map.json`). Simple est le défaut ; Réaliste charge `web/public/models/CarConcept.glb`
(« Car Concept », © 2024 Darmstadt Graphics Group GmbH, Eric Chadwick, **CC-BY 4.0**), téléchargé par
`make models` et jamais commité. Gagne ici parce qu'une seule table sert les deux rendus et qu'aucune
donnée de BOM n'est inventée.

## Conséquences
- Honnêteté : badge « formes GÉNÉRÉES » (Simple), « SIMILAIRE — pas le modèle exact du véhicule » + attribution
  (Réaliste) ; nom inconnu → objet générique « position estimée » dans la zone de son système ; nœud sans
  équivalent réaliste → compté et signalé, pas caché. Logos Khronos masqués (plaque, flancs, emblème volant).
- Repli : modèle absent ou illisible → Simple + message. Couleur : `budget_status` (`ok|warn|over`) lu sur
  les nœuds si l'API le fournit ; sinon couleurs par système et avis explicite (la règle budget reste au back,
  ADR 0018). Statut absent sur un nœud → gris « indisponible », jamais vert.
- Perf : rendu à la demande (pas de boucle au repos), instancing (sièges, géométries de roue partagées),
  ressources libérées au démontage, pas de post-traitement ; compression meshopt si `gltfpack` est présent
  (décodeur inclus dans `three`). Une seule dépendance runtime : `three` (+ `@types/three` en dev).
- Difficile : un nouveau véhicule (Valhalla) a des noms différents → enrichir `slots.json`. Retour arrière :
  retirer le composant ; aucun schéma ni API touché. Pivot hors-ligne : le modèle est un fichier statique
  servi localement, compatible ; multi-usine/carbone : sans effet, la 3D ne calcule rien.
