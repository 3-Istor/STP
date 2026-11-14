# 0022 — Stock par BOM et couverture des pièces
Statut : Accepté — Date : 2026-11-14 — Décideurs : Arthur.presle (accord explicite dans le chat)

## Contexte
stock-and-reuse (`docs/features.md:302–319`) : stock par pièce **propre à chaque BOM**, couverture
totale ou partielle affichée, délai nul si la quantité est suffisante. La spec ne dit pas : à quelle
quantité on compare le stock, comment un même stock se répartit entre plusieurs nœuds de la même pièce,
ni ce que devient le coût d'une pièce couverte. Ce sont des règles de calcul absentes (AGENTS §7), et le
stock doit être figé à la publication (modèle de snapshot, ADR 0006).

## Options
1. Colonne `stock_quantity` sur `bom_nodes` — simple ; mais le stock est par pièce, pas par nœud :
   deux nœuds de la même pièce auraient deux stocks.
2. Table `bom_part_stock (bom_id, part_id, quantity NUMERIC ≥ 0)` + copie `bom_snapshot_part_stock`
   à la publication — un stock par (BOM, pièce), conforme à la spec ; une table et une copie de plus.
3. Ne rien faire — la feature reste absente.

## Décision
**option 2.** Le stock est une donnée de la BOM, pas du catalogue ; la copie au snapshot
garde la reproductibilité. Le calcul reste dans `bom.ComputeTotals` (pur, sans base).

## Conséquences
- Quantité comparée : **quantité réelle × volume de la BOM** (le stock est physique).
- Répartition d'un stock entre nœuds de la même pièce : par ordre d'id de nœud croissant (déterministe).
- Couverture totale → délai propre du nœud = 0 (ni livraison ni production) et marque « en stock » ;
  partielle → délai de l'offre conservé, quantité couverte affichée.
- Coût d'une quantité couverte : **inchangé** (le stock n'est pas valorisé à zéro sans règle explicite).
- Ancien snapshot sans stock : lu comme « aucun stock », sans backfill.
- Retour arrière : migration down supprimant les deux tables ; aucun calcul existant modifié sans stock.
- Pivot multi-usine : la clé deviendrait (BOM, usine, pièce) ; la table s'étend sans casser l'API.
