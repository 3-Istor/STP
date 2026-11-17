# 0034 — Navigation en 5 étapes, routeur maison et thème clair / sombre
Statut : Proposé — Date : 2026-11-17 — Décideurs : à compléter (un membre de l'équipe suffit)

## Contexte
La V1 a deux espaces (BOM, Fournisseurs) avec deux mises en page et deux feuilles de style ; l'état de navigation
tient dans une ancre (`#boms`, `#fournisseurs`) et dans l'état React (BOM ouverte, nœud sélectionné), perdu au
rechargement. La vision V2 (`docs/v2/vision-cible.md` §4, maquettes I4) impose un parcours en 5 étapes
(Structurer, Chiffrer, Sourcer, Arbitrer, Publier) où la **même sélection** suit d'une étape à l'autre, une colonne
« Synthèse » commune et un thème clair / sombre enregistré par navigateur. Règle du lot C : aucune dépendance
ajoutée sans ADR, aucune fonction existante perdue.

## Options
1. **Routeur maison sur l'ancre (`#/boms/1/structurer?node=8437`)** — ~100 lignes pures (`parseRoute` /
   `formatRoute`, testées sans navigateur) + `hashchange`. Aucune dépendance, fonctionne derrière le nginx actuel
   sans réécriture d'URL, anciennes ancres redirigées. Inconvénient : pas de chargement de données par route,
   URL avec `#`.
2. **React Router (ou TanStack Router)** — riche (loaders, routes imbriquées), mais dépendance npm à maintenir et
   à auditer (`npm audit` en CI) pour 6 routes ; l'History API demande aussi une règle de repli côté nginx.
3. **History API maison (`/boms/1/structurer`)** — URL plus propres, mais repli nginx et serveur Vite à régler,
   et les liens profonds échouent si la règle manque.
4. **Ne rien faire** (état React seul) — ni lien profond, ni retour arrière, ni sélection partagée fiable.

Thème : (a) **variables CSS dans une seule feuille de tokens** (`web/src/ui/tokens.css`), `data-theme` posé sur
`<html>`, préférence dans `localStorage` (try/catch), `prefers-color-scheme` par défaut ; (b) deux feuilles
complètes ; (c) bibliothèque de thèmes. (a) seule ne duplique rien et garde `--ok --warn --over` (AGENTS §5)
comme source unique.

## Décision
Option 1 + thème (a) : le routeur sur l'ancre couvre les besoins (liens profonds, retour arrière, sélection
partagée) sans dépendance ni configuration serveur ; la sélection d'un nœud remplace l'entrée d'historique
(pas d'historique saturé par les clics), un changement d'étape en ajoute une.

## Conséquences
- Facile : partager un lien vers une étape et un nœud ; brancher la vue 3D du lot D (`Vehicle3DSlot` :
  `bomId`, `nodes`, `selectedNodeId`, `onSelectNode`) ; ajouter une étape ou un onglet (`?tab=`).
- Difficile : chargement par route, routes imbriquées profondes ; si le besoin apparaît, l'option 2 remplace
  `router.ts` sans toucher aux étapes (elles ne reçoivent qu'une `Route`).
- Retour arrière : `router.ts` et `ui/theme.ts` sont isolés ; les couleurs restent des variables.
- Barres fines de la Synthèse : largeur = part du coût du véhicule, calculée en entiers exacts (BigInt),
  géométrie d'affichage seulement ; le « % du budget » des maquettes attend les budgets (lot E).
- Pivot : hors-ligne strict — l'URL ne dépend d'aucun serveur ; multi-source de prix et multi-usine — l'onglet et
  la sélection passent dans l'URL sans changer le routeur ; carbone versionné — sans impact.
