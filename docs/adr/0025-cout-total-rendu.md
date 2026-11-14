# 0025 — Coût total rendu (douane, transport, emballage)
Statut : Accepté — Date : 2026-11-14 — Décideurs : Arthur.presle (accord explicite dans le chat)

## Contexte
total-landed-cost (`docs/features.md:360–378`) : coût rendu = achat + douane (pays d'origine et de
destination) + transport + emballage ; tables paramétrables ; comparaison côte à côte de deux offres.
Les offres ont un `origin_country` ; aucun pays de **destination** n'existe. La spec ne dit pas la base
de chaque composante (pourcentage, par unité, par kg), ni si les totaux de BOM passent au coût rendu.

## Options
1. Nouveau domaine `internal/landedcost` (tables `customs_duties`, `freight_rates`, réglage
   `destination_country` sur la BOM) — frontière nette ; un domaine de plus (AGENTS §7).
2. Tables et calcul dans `pricing` — pas de nouveau domaine ; `pricing` grossit (cotations + logistique).
3. Composantes saisies sur chaque offre — pas de table de pays ; contraire à « tables paramétrables
   selon origine et destination ».

## Décision
**option 2**, `pricing` possède déjà la conversion et la provenance des montants ; un
évaluateur pur `LandedCost(offre, destination, tables, prix, taux)` renvoie chaque composante.

## Conséquences
- Douane : taux en % (`NUMERIC`) par (origine, destination), appliqué au prix d'achat.
- Transport : montant + devise **par kg** par (origine, destination), × masse de la pièce.
- Emballage : montant + devise **par unité**, porté par l'offre (nouvelle colonne).
- Chaque composante convertie dans la devise de la BOM avec les taux du même contexte (direct ou snapshot) ;
  ligne de table manquante → erreur nommée (pas de douane à 0 implicite).
- Destination : réglage `destination_country CHAR(2)` de la BOM, copié au snapshot (ADR 0006 étendue).
- Les totaux de BOM **restent au prix d'achat** ; le coût rendu est un calcul à part (comparaison, make-or-buy).
- API : `GET /api/v1/parts/{id}/landed-cost?bom_id=&offer_ids=a,b` (comparaison côte à côte, détail).
- Pivot multi-usine : la destination devient l'usine ; multi-source de prix : inchangé (même adapter).
