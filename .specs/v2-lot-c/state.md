---
id: v2-lot-c
status: review
branch: v2-lot-c
current_task: null
started: 2026-11-17
updated: 2026-11-17
---

# État — V2 lot C (shell front en 5 étapes)

## Progress

T1 à T7 faits. Le front a une seule structure (barre du haut, étape, Synthèse, barre d'état) ; toutes les
fonctions de la V1 ont une place (carte ci-dessous). Reste à la relecture humaine : ADR 0034 (Proposé),
changements de comportement listés dans « Decisions ».

## Task Log

- 2026-11-17 T0 — spec et état créés après lecture d'AGENTS.md, du plan, de la vision §4, des 9 maquettes
  (png + dc.html), de `web/src` et `web/e2e`.
- 2026-11-17 T1 — `web/src/ui/tokens.css` (seule feuille de couleurs, thème sombre par `[data-theme='dark']`),
  `web/src/ui/theme.ts` (préférence `localStorage` en try/catch, `prefers-color-scheme` par défaut, suivi du
  système tant que rien n'est choisi). `index.css` et `bom.css` réécrites en `var(--…)` seulement.
- 2026-11-17 T2 — `web/src/router.ts` (hash, `parseRoute`/`formatRoute` purs, `useRoute`, réécriture des ancres
  V1 sans entrée d'historique), `web/e2e/router.spec.ts`.
- 2026-11-17 T3 — shell (`App.tsx`, `shell/*`), Toutes les BOM (`home/AllBoms.tsx`), `bom/useBomData.ts`
  (brouillon ou snapshot, ex-BomView/SnapshotView), `bom/BomWorkspace.tsx`, étapes `bom/steps/*`,
  `NodePanel` réduit au détail (stratégie → Arbitrer ; coût rendu, make-or-buy → Chiffrer), `BomTree`
  (recherche extérieure, colonnes Budget et CO₂, actions en ligne, révélation du nœud sélectionné).
  Supprimés : `BomPage`, `BomList`, `BomView`, `SnapshotView`, `PriceSource`, `Totals`, `priceSnapshots.ts`.
- 2026-11-17 T4 — `suppliers/SuppliersPage.tsx` en tables denses, carte « Historique du prix » avec période
  (1 mois / 3 mois / 6 mois / Tout), fournisseur choisi dans l'URL ; `graph.ts` : regroupement jour / semaine /
  mois (`bucketSeries`) et filtre de période (`inPeriod`).
- 2026-11-17 T5 — e2e mis à jour (sélecteurs et parcours), nouveaux : `shell.spec.ts`, lien profond, suppression
  d'un nœud depuis l'arbre, courbe par période, fournisseur dans l'URL.
- 2026-11-17 T6 — `docs/adr/0034-navigation-5-etapes-routeur-theme.md` (Proposé).
- 2026-11-17 T7 — vérification sur serveur réel (seed demo-v3), captures clair / sombre dans
  `docs/v2/captures/lot-c/`.

## Carte « fonction existante → nouvelle place »

| Fonction V1 | Place V2 |
|---|---|
| Onglets BOM / Fournisseurs (en-tête) | Barre du haut : fil d'Ariane + 5 étapes ; Fournisseurs = Sourcer › onglet 2 (et `#/fournisseurs` hors BOM) |
| Liste des BOM (colonne gauche), ouvrir | Toutes les BOM (`#/boms`) : tableau, lien du nom ou « Ouvrir » |
| Créer une BOM (nom, devise, volume) | Toutes les BOM › « + Nouvelle BOM » (fenêtre) |
| Renommer, supprimer une BOM | Toutes les BOM › boutons de ligne (fenêtres de l'app) |
| Ouvrir un snapshot, nouveau brouillon depuis un snapshot | Toutes les BOM (ligne snapshot) ; étape Publier du snapshot |
| Lien « créé depuis le snapshot #N » | Toutes les BOM (« depuis #N ») ; Publier › Versions |
| Devise de référence, volume, pays de destination | Colonne droite › onglet « Réglages de calcul » |
| Base horaire, ETP par sous-système | Chiffrer › Main-d'œuvre (ETP total aussi dans la Synthèse) |
| Seuil et résumé des anomalies | Chiffrer › Anomalies ; badges dans l'arbre ; compte en badge de l'étape Chiffrer |
| Planning (départ, cible, chemin critique) | Chiffrer › Disponibilité et planning ; disponibilité vs cible dans la Synthèse |
| Totaux (coût, CO₂, délai, ETP) et « sur quoi ils reposent » | Synthèse (total, CO₂, masse, ETP, disponibilité, « Sur quoi repose ce total ») |
| Source des prix (en direct / snapshot, « Figer maintenant ») | Synthèse › Prix utilisés (boutons radio, dates à la seconde) |
| Publier | Étape Publier |
| Arbre virtualisé, recherche, changement d'offre en ligne | Structurer (colonnes Qté, Offre retenue, Coût/véh., Budget, CO₂, Délai) |
| Ajouter (pièce avec choix de l'offre / sous-ensemble) | Structurer › « + Ajouter une pièce », « + » sur la ligne ; aussi dans le Détail |
| Déplacer, supprimer un nœud | Structurer › « ⇅ » / « × » sur la ligne ; aussi dans le Détail |
| Détail du nœud (faits, explication, quantité, jours de production, stock) | Colonne droite › Détail, sur toutes les étapes |
| Stratégie du nœud et alternatives | Arbitrer |
| Coût rendu (comparer les origines) | Chiffrer › élément sélectionné (pièce) |
| Make or buy | Chiffrer › élément sélectionné (sous-ensemble) |
| Fournisseurs : création, filtre | Sourcer › Fournisseurs (table de gauche, « + Fournisseur ») |
| Offres, recherche, « Nouvelle pièce vendue » | Sourcer › table « Offres de … », « + Offre » (même fenêtre) |
| Historique d'une offre (fenêtre, clavier, tactile) | Sourcer › carte « Historique du prix » (période au choix) + « Toutes les cotations… » / « ↗ » (même fenêtre) |
| Contrats et remises, création, modification | Sourcer › table « Contrats et remises », « + Contrat », « Modifier » |
| Flux de prix en direct (SSE), Actualiser | Sourcer › en-tête du fournisseur |

## Decisions

- D1 Routeur maison sur l'ancre, thème par variables : ADR 0034 (Proposé).
- D2 Changement de comportement assumé : dans « Toutes les BOM », un clic sur la ligne affiche l'aperçu (maquette
  I4-0) ; l'ouverture passe par le nom ou « Ouvrir ». Le test e2e correspondant vérifie le nouveau comportement.
- D3 Le sélecteur de source de prix devient un groupe de boutons radio dans la Synthèse ; 3 snapshots montrés,
  les autres via « Tous les snapshots (N)… », le snapshot attaché toujours visible.
- D4 Barres fines de la Synthèse : part du coût du véhicule (BigInt, géométrie seule, aucun pourcentage affiché)
  au lieu du « % du budget » (aucun budget dans l'API).
- D5 Colonne « Budget » de l'arbre et « Hors budget » : « — » avec une infobulle (lot E), jamais de chiffre.
- D6 Prix des offres (Sourcer) dans la devise de l'offre, comme la V1 ; la courbe est en EUR quand l'API la
  convertit (ADR 0030). Périodes comptées depuis `statistics.as_of` (jamais l'heure du poste).
- D7 IBM Plex Sans utilisée seulement si installée (aucune police téléchargée) ; repli police système.
- D8 Pas de liste des snapshots publiés côté API : « Toutes les BOM » et « Publier › Versions » montrent ceux à
  l'origine d'un brouillon (GET /snapshots/{id}, lu une fois) et ceux publiés depuis l'onglet.

## Blockers

Aucun. Endpoints manquants (aucun changement d'API fait, à livrer par d'autres lots) :
- liste des versions d'une BOM (snapshots publiés, coût, % budget) → lot B (`GET /boms/{id}/versions`) ;
- budgets par BOM et par sous-système → lot E ; méthode de calcul → lot A (`cost_method`, affichée dès qu'elle
  est renvoyée dans l'explication de l'arbre) ;
- en-tête léger d'un snapshot (`GET /snapshots/{id}` renvoie tout l'arbre figé, lourd pour la liste) ;
- nombre de cotations d'un snapshot de prix (affiché par la maquette) absent de `GET /price-snapshots`.

## Verification Report(s)

### 2026-11-17 — rapport T7

Environnement : base `stp_lot_c` (goose up → 00011), `server seed` (demo-v3, `SIM_*` du Makefile/compose),
API de prix `:8092`, serveur `:8082`, Vite `:5182` (`STP_API_PROXY=http://localhost:8082`).

| Vérification | Résultat |
|---|---|
| `npm ci` | PASS (0 vulnérabilité) |
| `npm run lint` (oxlint) | PASS, 0 avertissement |
| `npx tsc -b` | PASS |
| `npx vite build` | PASS |
| Playwright, projets `chromium-desktop` + `chromium-mobile` (Chrome système `/usr/bin/google-chrome` via une config locale non commitée) | PASS, 74/74 |
| Playwright `firefox-*`, `webkit-*` | NON EXÉCUTÉ : navigateurs Playwright absents de la machine (CI seule) |
| `go vet ./...`, `go test ./...` (avec `DATABASE_URL`) | PASS |
| `golangci-lint` | NON EXÉCUTÉ (aucun fichier Go modifié) |
| Parcours réel (script Playwright sur le seed) : créer une BOM, ajouter un sous-ensemble, ajouter une pièce avec choix de l'offre, changer d'offre, déplacer, figer puis repasser en direct, Chiffrer (make-or-buy, planning), Arbitrer (stratégie), supprimer avec confirmation, publier, ouvrir le snapshot, nouveau brouillon, liste, Sourcer (contrat, période 3 mois), thème rechargé | PASS (BOM d'essai supprimées ensuite, sauf 2 qui ont un snapshot publié : suppression refusée par l'API, 409) |
| Captures clair et sombre de chaque écran (1440×900) relues | `docs/v2/captures/lot-c/` (20 fichiers) |

Écarts restants avec les maquettes : budget, % budget, hors budget, causes d'écart, contrôles de publication,
analyses de Sourcer et Chiffrer, profil et méthode (encarts « lot A/B/E ») ; pas d'annuler / rétablir ni de
« Colonnes ▾ » (fonctions inexistantes) ; badges d'étapes seulement là où une donnée existe (nœuds, anomalies).
