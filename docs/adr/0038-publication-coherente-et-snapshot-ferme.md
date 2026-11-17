# 0038 — Publication cohérente, snapshot de BOM fermé au commit et budgets figés
Statut : Proposé — Date : 2026-11-17 — Décideurs : (à compléter par un humain)

## Contexte
Relecture indépendante du backend V2 (ticket `v2-lot-g`). AGENTS §3 : un snapshot copie structure, quantités,
**réglages**, prix et taux, et ne se modifie jamais (service **et** base). Trois écarts : (1) `Publish` lisait le
brouillon trois fois (contrôles, arbre, coût) hors transaction : un PATCH concurrent pouvait figer des nœuds
qui gardent l'offre A et des faits de coût de l'offre B (`ErrNoCostFacts` à vie), ou un nœud jamais contrôlé ;
(2) les triggers `reject_mutation` refusent UPDATE/DELETE/TRUNCATE mais pas un INSERT tardif dans une table
figée ; (3) les budgets cibles n'étaient pas figés : le % du budget de V1 et les budgets d'un brouillon issu de
V1 suivaient le budget actuel de la BOM.

## Options
1. **Ne rien faire** — rien à coder ; snapshots incohérents ou « modifiables » par ajout, % de V1 qui bouge.
2. **Verrou** `SELECT … FOR UPDATE` sur la ligne `boms` pendant toute la publication, pris aussi par chaque
   écriture de nœud — sûr, mais toutes les écritures de brouillon (≈ 20 méthodes du store) doivent le prendre,
   et la publication (gel des prix via un autre service, 30 000 nœuds) bloque l'édition.
3. **Une seule lecture cohérente** (`bom.Store.Draft` : BOM, arbre, stock, planning, budgets dans une
   transaction REPEATABLE READ en lecture seule) passée aux contrôles, au gel des nœuds et aux faits de coût ;
   **fermeture au commit** des tables figées comme les snapshots de prix (ADR 0016) ; **budgets figés** dans
   `bom_snapshot_target_budgets`.

## Décision
**Option 3.** Le snapshot est la photo d'un instant du brouillon : il suffit que toutes ses parties viennent du
même instant, sans bloquer l'édition. Un PATCH validé pendant la publication n'est simplement pas dans le
snapshot (il le sera au suivant). Base : `bom_snapshots.creation_xid` posé à l'insertion ; un trigger
d'instruction (table de transition, une requête par COPY) sur chaque table figée refuse toute ligne d'un
snapshot créé par une autre transaction (`restrict_violation`). Budgets : copiés par clé de nœud figé ;
`budgets_frozen = false` pour un snapshot publié avant 00015 (ses budgets n'existent plus à l'identique : on
garde l'ancien comportement, budgets actuels de la BOM, nommé dans le code). Rejeu : seul le taux budget de
l'année de `cost_as_of` vers la devise du snapshot est lu. Pourcentage du budget : une seule fonction exacte du
domaine (`bom.PercentOf`), un seul arrondi half-even en sortie (ADR 0004).

## Conséquences
- Facile : contrôles, nœuds et coûts figés sont cohérents par construction ; un INSERT tardif (même en psql)
  est refusé ; % de V1 et brouillon issu de V1 ne bougent plus quand le budget de la BOM change.
- Difficile : une publication longue lit un état qui peut être dépassé de quelques secondes (assumé) ; en prix
  en direct, le même état est chiffré deux fois (prix en direct pour les contrôles, prix figés pour le gel) ;
  un test qui veut écrire dans un snapshot commité doit le faire dans la transaction de création.
- Retour arrière : migration 00015 `Down` (colonnes, triggers, table) ; `Store.Draft` peut redevenir trois
  lectures sans autre changement.
- Pivot : multi-source de prix et carbone versionné ajoutent des tables figées — chacune prend le trigger
  `guard_bom_snapshot_rows` ; hors-ligne : la lecture cohérente est locale, sans réseau.
