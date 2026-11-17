# 0048 — Le gel de prix d'une BOM porte sur tout le catalogue
Statut : Accepté — Date : 2026-11-17 — Décideurs : Arthur.presle (accord explicite dans le chat)

## Contexte
Constat sur la BOM 3 (base de dev) : « Figer maintenant » crée un lot de prix de portée `bom`
(`internal/pricing/bom_pricing.go`, `Scope: "bom"`), qui ne contient que les offres utilisées par les nœuds au moment du
gel (257 offres pour 1 018 au catalogue). Le nœud 50105 a ensuite changé d'offre (131 → 130) : l'offre 130 n'est pas dans
le lot #404, la BOM lit ses prix dans le lot et la pièce n'a plus de prix (`no_quote`, « offer 130 in price snapshot 404 »).
Trois ensembles au-dessus sont sans coût, le budget global est « n/d », la BOM est « incomplète ». Aucune cotation ne
manque : seul le périmètre du gel est trop étroit. L'équipe veut qu'un gel contienne tous les prix de l'API de prix.
Le service de lots sait déjà créer une portée `catalog` (toutes les offres et leurs taux vers les devises cibles).

## Options
1. **Ne rien faire** — changer d'offre après le gel rend la BOM incomplète ; la publication refuse (`checkCovered`) tant
   qu'on ne refige pas.
2. **Gel de portée `catalog`** — `FreezePrices` (gel manuel et publication sans lot) crée un lot de toute la grille de
   prix : une cotation par offre du catalogue et les taux de chaque devise d'offre vers la devise de la BOM.
3. **Lot catalogue partagé** — un lot par instant de la source, réutilisé par toutes les BOM : moins de lignes, mais un
   lot partagé couple les BOM entre elles et complique « figer maintenant » et la suppression d'une BOM.
4. **Refiger en silence au changement d'offre** — corrige le symptôme en changeant les prix de la BOM sans que
   personne ne l'ait demandé. Écartée : une BOM figée ne change pas ses prix sans action visible.

## Décision
**Option 2.**
- `BOMPricing.FreezePrices` demande un lot de portée `catalog` aux devises cibles de la BOM. La vérification « le lot
  couvre les offres attendues » reste, mais elle est vraie par construction.
- Les taux des **devises de budget** (ADR 0044) restent figés avec le lot : la requête de lot reçoit ces devises
  comme sources supplémentaires (aujourd'hui lues par la portée `bom`, absentes de la portée `catalog`).
- Le gel est refusé, sans rien écrire, si une offre du catalogue n'a pas de cotation ou si une paire de taux manque :
  l'erreur liste tout ce qui manque (`CoverageError`), comme aujourd'hui.
- Les lots déjà créés (portée `bom`) restent lisibles et immuables ; une BOM qui en utilise un garde son comportement
  jusqu'à ce qu'on la refige.
- Publication (`snapshot.Publish`) : inchangée, elle réutilise le lot du brouillon (`checkCovered`) ou en fige un.

## Conséquences
- Facile : changer l'offre d'un nœud, ou ajouter une pièce, ne casse plus le chiffrage d'une BOM figée ; un seul
  « Figer maintenant » couvre tout le catalogue ; un nouveau brouillon copié d'un snapshot lit déjà un lot complet.
- Difficile : un gel est refusé dès qu'**une** offre du catalogue (même hors BOM) n'a pas de cotation ou de taux à la
  date ; chaque lot écrit autant de liens que d'offres (≈ 1 000 ici, pas de copie des cotations) ; le gel est plus
  long. Un lot catalogue prend l'instant courant de la source (temps simulé), comme aujourd'hui.
- « Même entrée + même snapshot = même résultat » : inchangé, les lots restent immuables.
- Retour arrière : repasser `Scope` à `bom` dans `FreezePrices` (aucune migration, les lots existants ne changent pas).
- Niveau de risque 3 (`internal/pricing` snapshot, publication) : plan, tests d'intégration (gel d'une BOM puis
  changement d'offre, gel refusé sur offre sans cotation, devises de budget) et relecteurs sécurité et métier.
