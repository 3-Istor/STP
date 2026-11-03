---
id: nlpf-69
status: review
branch: nlpf-69
current_task: null
started: 2026-11-02
updated: 2026-11-03
---

# State: BOM (C)

## Progress

All tasks completed. Ready for verification (`/spec-verify nlpf-69` non lancé, à la demande d'Arthur.presle).
- Tasks: 9/9 completed
- Branch: `nlpf-69`
- Commits: 914db9b (démo BOM_DEMO=1), 71d67a3 (T9), plus suivi ; rien de poussé.

## Task Log

### T1: ADR 0006 (stockage snapshot de BOM) et ADR 0007 (règles de calcul des totaux)
- **Status**: completed
- **Time**: 2026-11-02
- **Commit**: 737be8f
- **Notes**: Deux ADR au statut `Proposé`. 0007 tranche aussi, en plus de D4, le comportement d'un parent
  quand un prix/taux manque (pas de total de coût, liste des nœuds en défaut ; CO2 et délai restent calculés)
  et inclut la masse avec la même règle que le CO2 : à valider par l'humain.

### T2: Étendre `catalog.Part` et `pricing.Pricing` (+ stubs)
- **Status**: completed
- **Time**: 2026-11-02
- **Commit**: 82548e3
- **Notes**: `Part` + `CO2Kg`, `MassKg`, `ProductionDays` (stub refuse les valeurs négatives).
  `Pricing` + `FreezePrices(ctx, offerIDs, currencies, at)`, `SnapshotOfferPrice`, `SnapshotRate`,
  sentinelle `ErrPriceSnapshotNotFound`. Stub rendu mutable (`AddQuote`, `AddRate`, mutex) pour simuler
  l'arrivée de cotations. Contrat d'interface partagé : à annoncer à Joe (NLPF-67, NLPF-68).

### T3: Migration 00002 — colonnes brouillon et tables de snapshot de BOM
- **Status**: completed
- **Time**: 2026-11-02
- **Commit**: 0e6c472
- **Notes**: `bom_nodes.production_days`, `boms.price_snapshot_id` / `source_snapshot_id`, `bom_snapshots`
  (FK `bom_id` sans cascade, `price_snapshot_id` NOT NULL) et `bom_snapshot_nodes` (PK `(snapshot_id, node_key)`,
  FK parent **différée** pour insérer dans n'importe quel ordre, une seule racine, CHECK : un nœud pièce a
  CO2/masse/délai de livraison figés, un sous-ensemble n'en a pas). Triggers `reject_mutation`.
  `goose up/reset/up` vérifié sur une base jetable (`bomstudio_t3_check`, supprimée ensuite) : `reset` sur la
  base de dev a été refusé par le garde-fou (destruction des données locales). Test d'intégration : refus
  UPDATE/DELETE/TRUNCATE (23001), DELETE d'une BOM publiée (23503), brouillon toujours modifiable, lignes figées invalides refusées.

### T4: Accès données BOM (sqlc) et opérations d'arbre
- **Status**: completed
- **Time**: 2026-11-02
- **Commit**: 52d0865
- **Notes**: Option (a) : `internal/bom/store.go` = interface `Store`, types `BOM`/`Node`, sentinelles
  (`ErrBOMNotFound`, `ErrNodeNotFound`, `ErrCycle`, `ErrRootNode`, `ErrHasSnapshots`, `ErrNotAPart`) ;
  implémentation pgx dans `internal/bom/bomstore` (au lieu de `internal/bom/store.go` prévu par la spec),
  requêtes `internal/bom/queries.sql` → `internal/bom/bomdb`. `MoveNode` verrouille la ligne `boms`
  (`FOR UPDATE`) avant la CTE pour que deux déplacements concurrents ne créent pas de cycle.
  Ajouts hors liste de la spec, utiles à T6/T8 : `SetNodeQuantity`, `SetNodeProductionDays`, `SetPriceSnapshot`.
  sqlc refuse `EXISTS (SELECT … FROM cte)` : contrôle écrit avec `bool_or`. Modèles `catalogdb`/`pricingdb`
  régénérés (T3 les avait laissés périmés : la CI « code sqlc à jour » aurait échoué).
  Deadlock 40P01 entre le `TRUNCATE bom_snapshots CASCADE` du test T3 et les nouveaux tests quand les paquets
  tournent en parallèle : verrou `pg_advisory_xact_lock(69)` dans le `testDB` des deux paquets (5 runs verts).
  Mutation vérifiée : sans le contrôle de cycle, 4 cas de `TestMoveNode` échouent.

### T5: Calcul des totaux en Go (pur, sans base) + benchmark
- **Status**: completed
- **Time**: 2026-11-02
- **Commit**: b0cfcb2
- **Notes**: Signature `ComputeTotals(TotalsInput)` (struct au lieu de 4 paramètres) avec `CalcNode` détaché
  du brouillon, pour que T7 calcule un snapshot avec ses attributs figés. Les 4 exemples de R6, `ErrNoQuote`,
  `ErrNoRate`, taux de la mauvaise paire, absence d'arrondi intermédiaire, déterminisme et explication
  (taux utilisés triés, dates, source) testés. Benchmark : ~35 ms/op pour 30 000 nœuds.
  Choix agent à valider : une pièce qui a des enfants rend l'arbre invalide (`ErrInvalidTree`) plutôt que
  d'inventer une règle de cumul ; T6 devra refuser d'ajouter/déplacer un nœud sous une pièce.

### ADR 0008 : adaptation de `ComputeTotals` (préalable à T6)
- **Status**: completed
- **Time**: 2026-11-03
- **Commit**: 68251ba (ADR acceptée : 388af69)
- **Notes**: Rejet `ErrInvalidTree` « part node has children » retiré ; chaque nœud = contribution propre
  (pièce) + enfants. Nouvelle erreur de coût : une pièce sans prix garde son `ErrNoQuote`/`ErrNoRate` même si
  ses enfants en manquent aussi, et se liste en tête de `MissingCost`. `NodeTotals` gagne `OwnCost` et
  `OwnLeadTimeDays` (panneau latéral, ADR 0008 « Conséquences »). Tests : exemple Roue/Écrou de l'ADR
  (105 €/roue, 420 €, 20 écrous, 35 j), prix manquant sur parent, enfant, les deux, pièce racine.
  Benchmark 30k : ~28 ms/op.

### T6: Service BOM — CRUD, arbre, prix en direct ou figés, totaux
- **Status**: completed
- **Time**: 2026-11-03
- **Commit**: 38fe974
- **Notes**: `NewService(store, catalog, pricing)`. Sentinelles ajoutées : `ErrInvalidInput`, `ErrOfferNotForPart`.
  `FreezePrices(ctx, bomID, at)` (instant fourni par l'appelant) / `UnfreezePrices`. `Totals` renvoie
  `TreeTotals{BOM, Nodes, Totals}` pour T8. Tests sur un `Store` en mémoire (`memstore_test.go`, fichier de test
  hors liste de la spec). Mutations vérifiées : sans contrôle offre/pièce, toujours en direct, ou production du
  catalogue ignorée → au moins un test échoue. Tests d'intégration Postgres **sautés en local** (la base sur
  :5432 refuse l'utilisateur `bom`, aucun conteneur du projet lancé) : T6 ne touche pas la base, à rejouer en CI.

### T7: Domaine snapshot — publier, lire, nouveau brouillon
- **Status**: completed
- **Time**: 2026-11-03
- **Commit**: d82fb86
- **Notes**: `snapshot.Service{Publish(ctx, bomID, at), Get, NewDraft}` ; `Store` sans méthode de modification.
  Implémentation pgx dans `internal/snapshot/snapshotstore` (hors liste de la spec, même découpage que
  `bom/bomstore`, option (a) de T4) ; nœuds insérés par un `COPY` (`:copyfrom`), FK parent différée vérifiée
  au commit. Dans `bom` : `Service.Tree` (BOM + nœuds) et `LoadPrices` exportée (extraite de `totalsInput`,
  partagée par les totaux d'un brouillon et d'un snapshot ; tests T6 inchangés et verts).
  Tests d'intégration (Postgres, **exécutés** : stack `compose.yaml`, service `db` remappé sur :5433 par un
  override local hors dépôt, :5432 étant pris par une autre instance) : CO2/production du catalogue,
  nouvelle cotation, nouveau taux, quantité et suppression dans le brouillon → totaux du snapshot inchangés
  (388,00 €, 42 kg CO2, 32,2 kg, 35 j) ; snapshot de prix créé ou réutilisé ; refus si prix/taux manquant ;
  brouillon depuis snapshot = même arbre, pièces, offres, quantités, volume, devise, `source_snapshot_id` ;
  brouillon partiel supprimé si la copie échoue ; `DeleteBOM` publiée → `ErrHasSnapshots` (FK réelle).
  Mutations vérifiées (toujours re-figer, sans contrôle de couverture, sans compensation, `Get` en direct,
  production pièce non copiée) : chaque fois au moins un test échoue. Les tests T3/T4/T6 sautés jusqu'ici
  passent aussi sur cette base (aucun SKIP, `-race` vert, 3 runs).

### T8: API HTTP `/api/v1/boms` et câblage
- **Status**: completed
- **Time**: 2026-11-03
- **Commit**: edaff84
- **Notes**: Routes de la spec, handlers fins dans `internal/httpapi/boms.go` ; représentations JSON dans
  `internal/httpapi/boms_json.go` (fichier hors liste de la spec). `NewRouter(Deps{BOMs, Snapshots, Now})` :
  services vus par des interfaces `BOMService` / `SnapshotService` (satisfaites par `*bom.Service` et
  `*snapshot.Service`, vérifié à la compilation) ; l'instant d'un gel/d'une publication vient du routeur.
  Mapping : not found → 404 ; `ErrCycle`, `ErrHasSnapshots`, `ErrRootNode`, `ErrNoQuote`, `ErrNoRate` → 409 ;
  validation, JSON invalide, champ inconnu, id non numérique, offre/pièce inconnue du catalogue → 400 ; reste → 500
  « internal error » (cause journalisée par slog, pas renvoyée). `cmd/server` : pool pgx sur `DATABASE_URL`
  (ping au démarrage), stores Postgres, catalogue et prix = **stubs vides** (NLPF-67/68).
  Tests `httptest` (fakes des services, totaux produits par le vrai `ComputeTotals`) : chaque route, chaque code,
  erreur non avalée sur chaque route, corps de l'arbre (100 USD × 0,92 → `"92.00"` EUR, prix manquant → `cost`
  null + `no_quote`, `missing_cost`), corps d'un snapshot. Mutations vérifiées (7 : 409→400, id 0 accepté, PATCH
  multi-champs, 500 qui divulgue, champs inconnus, `missing_cost` null, code d'erreur) : toutes tuées.
  `go test -race ./...` vert sur Postgres :5433, aucun SKIP ; `golangci-lint` 0 issue ; `go mod tidy -diff` vide.
  Smoke test du vrai serveur sur la base : création, sous-ensembles, cycle 409, racine 409, offre inconnue 400,
  arbre, snapshot inconnu 404, suppression 204 ; **publier → 500** (FK `bom_snapshots_price_snapshot_id_fkey` :
  le snapshot de prix du stub n'existe pas en base), limite attendue tant que NLPF-68 n'est pas livré.

### Démo BOM_DEMO=1 (préalable à T9, demandé par Arthur.presle)
- **Status**: completed
- **Time**: 2026-11-03
- **Commit**: 914db9b
- **Notes**: `internal/demo` = seule source des données de démo (4 fournisseurs, 10 pièces, 13 offres EUR/USD,
  12 cotations, taux USD→EUR 0,92 et EUR→USD 1,087, ids 9 000 001+, cotations à 2026-01-05T08:00Z, source `demo`).
  `Seed` insère en base de façon idempotente (`OVERRIDING SYSTEM VALUE … ON CONFLICT DO NOTHING`, puis vérifie
  qu'un id déjà présent porte bien la même donnée, sinon erreur) ; `Catalog()` / `NewPricing()` construisent les
  stubs avec les mêmes ids. `demo.Pricing` = `pricing.Stub` pour les prix en direct + `FreezePrices` /
  `SnapshotOfferPrice` / `SnapshotRate` **en base** (`price_snapshots` + liens) : figer et publier passent la FK,
  et un snapshot publié reste lisible après redémarrage. Domaine `pricing` non modifié. `GET /api/v1/demo/catalog`
  (pièces + offres + cotation courante) monté dans `cmd/server` seulement en démo. Toggle : `BOM_DEMO=1` exactement,
  jamais par défaut ; `compose.yaml` passe `BOM_DEMO: ${BOM_DEMO:-}`. Pas de `make seed`, pas de Fiat Multipla.
  Tests : cohérence du jeu (devise, bornes, fournisseurs), handler (montant en chaîne, `price` null), `Seed` deux
  fois, id pris par une autre donnée refusé, gel + relecture, gel refusé sans rien créer si prix ou taux manquant.
  Smoke : 4 × 108,50 USD × 0,92 = 399,28 € ; figer → snapshot de prix #275 ; publier → snapshot #207 (201).

### T9: UI — liste des BOM, arbre virtualisé, panneau latéral
- **Status**: completed
- **Time**: 2026-11-03
- **Commit**: 71d67a3
- **Notes**: Fichiers de la spec + `web/src/catalog.ts` (catalogue de démo, isolé pour être remplacé par l'API
  NLPF-67), `web/src/bom/BomView.tsx`, `SnapshotView.tsx` (S2), `Totals.tsx`, `flatten.ts` (aplatissement itératif,
  enfants triés par id). Virtualisation : lignes de 30 px, fenêtre visible + 8 lignes de marge. Montants :
  `Intl.NumberFormat(..., { roundingMode: 'halfEven' })` sur la chaîne ; quantité commandée = réelle × volume en
  BigInt (S1). Déplacer = « Déplacer… » puis clic sur le nouveau parent ; supprimer = `confirm()`. Après chaque
  action, l'arbre est relu (pas de recalcul côté front). `npx oxlint` 0 avertissement, `npm run build` vert,
  `make lint` 0 issue, `go test -race ./...` vert.
  **Vérifié dans le navigateur** (Vite :5173 + serveur `BOM_DEMO=1` sur la base :5433) :
  - BOM générée de **30 031 nœuds** (30 modules × 10 sous-modules × 99 pièces, insérés en SQL) : 42 lignes max dans
    le DOM (337 éléments au total) sur 100 positions de scroll du haut en bas, aucune *long task* (> 50 ms) ;
    arbre de 15,8 Mo relu en ~370 ms, PATCH d'offre ~60 ms.
  - Changement d'offre (jante 108,50 USD → 100,00 EUR, ×2) : ligne 199,64 € → 200,00 €, racine
    6 764 071,92 € → 6 764 072,28 € (+0,36 €), délai 27 j → 12 j.
  - BOM manuelle : « Roues » (2 j) + jante 100 € × 4 → 400,00 € sur tous les parents, délai 12 j / 14 j ;
    déplacer « Roues » sous sa jante → message `move would create a cycle`, arbre inchangé ; déplacement valide ;
    suppression annulée puis confirmée (sous-arbre retiré, totaux recalculés) ; offre sans cotation → « prix
    manquant » en rouge (`--over`) sur la ligne et ses parents, gel refusé (409, message affiché) ; figer → publier →
    ouvrir le snapshot #208 (lecture seule, aucun `<select>`) → nouveau brouillon (« Créé depuis le snapshot #208 »,
    mêmes totaux, prix en direct). Console : seulement les deux 409 attendus.
  Non vérifié : le rendu visuel en direct (le panneau navigateur était masqué : vérifications faites par le DOM et
  des mesures, les captures étaient figées), le thème clair, la largeur mobile, Firefox/Safari.

## Decisions

- 2026-11-02 — D1 : snapshot de BOM en tables relationnelles (ADR 0006 à rédiger) — choix humain.
- 2026-11-02 — D2 : étendre `catalog.Part` (CO2, masse, délai de production) + stub dès maintenant, à annoncer à Joe (NLPF-67) — choix humain.
- 2026-11-02 — D3 : brouillon depuis snapshot = nouvelle BOM avec `source_snapshot_id` — choix humain.
- 2026-11-02 — D4 : délai feuille = livraison de l'offre + production de la pièce (ADR 0007 à rédiger) — choix humain.
- 2026-11-02 — Défaut agent : virtualisation de l'arbre sans dépendance npm ; figer les prix via extension de `pricing.Pricing` + stub (implémentation base : NLPF-68).
- 2026-11-02 — T2 (validé par Arthur.presle) : `currencies` de `FreezePrices` = devises **cibles** ; on fige le taux
  de chaque devise d'offre vers chacune d'elles (paire exacte).
- 2026-11-02 — T2 (validé par Arthur.presle) : `FreezePrices` **refuse** (rien n'est créé) s'il manque un prix ou un
  taux, et liste tout ce qui manque (`ErrNoQuote` / `ErrNoRate` joints). Alternative écartée : figer
  partiellement, ce qui graverait un trou dans un snapshot immuable.
- 2026-11-02 — T2 : le stub copie les cotations courantes et n'utilise pas `at` (pas d'historique) ;
  l'implémentation base (NLPF-68) devra figer la dernière cotation `quoted_at <= at`.
- 2026-11-02 — Point à trancher avant T4 : `internal/bom/service_test.go` contient
  `TestDomainHasNoForbiddenDependency`, qui interdit `github.com/jackc/pgx` dans les dépendances du package
  `internal/bom`. Or T4 place `store.go` (pgx + sqlc) dans ce même package. Options : (a) mettre le store
  dans un sous-package `internal/bom/bomstore` (le domaine reste sans pgx, le test inchangé) ;
  (b) restreindre le test (affaiblir un test : interdit sans accord, AGENTS §11). L'agent propose (a).
- 2026-11-02 — ADR 0006 et 0007 passées à `Accepté` par Arthur.presle (y compris, pour 0007, le parent sans
  total de coût quand un prix/taux manque et la masse traitée comme le CO2).
- 2026-11-02 — Point avant T4 tranché par Arthur.presle : option (a), store dans `internal/bom/bomstore`.
- 2026-11-02 — `.specs/CLAUDE.md` (règles du skill spec) n'existe pas ; exécution faite avec `AGENTS.md`.
- 2026-11-03 — Point avant T6 tranché par Arthur.presle : une pièce peut avoir des enfants (option (b), « sous tout
  nœud »). Règle de cumul rédigée en ADR 0008 (`Proposé`) : pièce parent = sa contribution propre + celle de ses
  enfants ; délai = max(enfants) + (livraison + production). Exemple : Roue 4 × 100 € + Écrou 5 × 1 € → 105 €/roue,
  420 €, 20 écrous, 35 j. Conséquence pour T6 : retirer le rejet `ErrInvalidTree` « part node has children » de
  `ComputeTotals` (T5) et ajouter les tests de l'exemple ; le service n'a rien à refuser.
- 2026-11-03 — ADR 0008 passée à `Accepté` par Arthur.presle.
- 2026-11-03 — T6, choix agent **validé par Arthur.presle** : les jours de production d'une **pièce** viennent du catalogue
  (`Part.ProductionDays`, ADR 0007 règle 4) ; le service refuse (`ErrInvalidInput`) de les saisir sur un nœud
  pièce plutôt que de stocker une valeur que le calcul ignorerait.
- 2026-11-03 — T6, choix agent **validé par Arthur.presle** : figer une BOM déjà figée la fige à nouveau sur les cotations
  courantes (nouveau snapshot de prix) ; l'ancien snapshot de prix reste en base, immuable.
- 2026-11-03 — T6, choix agent **validé par Arthur.presle** : `SetNodeOffer` / `SetNodeProductionDays` chargent l'arbre (une requête) pour
  connaître le type du nœud, faute de `GetNode` dans l'interface `Store` ; à remplacer si le coût se voit.
- 2026-11-03 — T6 : seuls `ErrNoQuote` / `ErrNoRate` deviennent une erreur sur le nœud ; un snapshot de prix
  introuvable, une pièce ou une offre absente du catalogue, ou une panne font échouer `Totals` (rien caché).
- 2026-11-03 — T7, choix agent **validé par Arthur.presle** : publier un brouillon **figé** réutilise son snapshot de prix, mais
  refuse (`ErrNoQuote` / `ErrNoRate`, rien n'est publié) s'il ne couvre plus toutes les offres ou la devise
  (offre ou devise changée après le gel) : même règle que `FreezePrices` (T2), pas de trou gravé dans un
  snapshot immuable. Alternative : re-figer automatiquement sur les cotations courantes.
- 2026-11-03 — T7, choix agent **validé par Arthur.presle** : un brouillon créé depuis un snapshot part en **prix en direct**
  (`price_snapshot_id` NULL) ; la spec (`docs/features.md` l. 182, 191) n'en dit rien. Alternative : l'attacher au snapshot de prix
  du snapshot (totaux identiques au départ), ce qui demande une méthode « attacher un snapshot de prix »
  côté `bom.Service` (aussi utile à « un snapshot de prix choisi », l. 179).
- 2026-11-03 — T7, choix agent **validé par Arthur.presle** : `NewDraft` passe par `bom.Service` (création + un `AddNode` par
  nœud, comme le demande la spec) ; ce n'est donc pas une transaction : en cas d'échec le brouillon partiel est
  supprimé (compensation). Coût : un aller-retour (et une lecture catalogue) par nœud, lent pour 30 000 nœuds.
- 2026-11-03 — T7 : la racine du nouveau brouillon porte le nom de la BOM (pas d'opération pour renommer un
  nœud) ; sa quantité et ses jours de production sont recopiés.
- 2026-11-03 — T7 : `published_at` = instant fourni par l'appelant (pas d'horloge dans le domaine) ; le
  `DEFAULT now()` de la colonne n'est pas utilisé.
- 2026-11-03 — T7 : les trois choix ci-dessus (refus de publier si le snapshot de prix ne couvre plus tout ;
  brouillon depuis snapshot en prix en direct ; `NewDraft` via `bom.Service` sans transaction, avec nettoyage)
  **validés tels quels par Arthur.presle**.
- 2026-11-03 — T8, choix agent **validé par Arthur.presle** : `PATCH /boms/{id}` prend **soit** `name`, **soit** `currency` **et**
  `volume` ensemble ; `PATCH /nodes/{nodeID}` prend **exactement un** champ (`offer_id`, `quantity`, `parent_id`,
  `production_days`). Chaque champ est une opération distincte du service : en accepter plusieurs appliquerait
  la requête à moitié si la deuxième échoue. Sinon 400.
- 2026-11-03 — T8, choix agent **validé par Arthur.presle** : montants renvoyés **exacts** (`money.Money`, ex. `"0.005"`), l'arrondi
  half-even se fait à l'affichage (ADR 0004, `Intl.NumberFormat` sur la chaîne en T9). Alternative : renvoyer
  aussi un montant arrondi par le back.
- 2026-11-03 — T8, choix agent **validé par Arthur.presle** : codes HTTP non fixés par la spec : `ErrRootNode`, `ErrNoQuote` /
  `ErrNoRate` (gel ou publication impossibles) → 409 ; offre/pièce inconnue du catalogue dans un corps → 400
  (et non 404, réservé à la ressource de l'URL) ; `cost_error_code` (`no_quote` / `no_rate` / `missing_cost`)
  ajouté pour que l'UI distingue les cas sans analyser le message.
- 2026-11-03 — T8, choix agent **validé par Arthur.presle** : le serveur **exige** `DATABASE_URL` et une base joignable au démarrage
  (il démarrait sans base jusqu'ici) ; `make run` / `compose.yaml` la fournissent déjà. Catalogue et prix câblés
  en stubs **vides** : pas de pièce ajoutable ni de gel/publication possible en vrai tant que NLPF-67/68 ne
  fournissent pas leurs implémentations Postgres. Le front (T9) ne pourra donc être vérifié de bout en bout
  qu'avec des sous-ensembles, ou avec un catalogue/prix de démonstration (à décider avant T9).
- 2026-11-03 — T9 : catalogue et prix de démo **demandés par Arthur.presle** (BOM_DEMO=1, `internal/demo`).
  Feature toggle — propriétaire : Arthur.presle ; raison : tester l'UI avant NLPF-67/68 ; **date de suppression :
  livraison de NLPF-67/68** (supprimer `internal/demo`, son câblage dans `cmd/server/main.go`, la ligne
  `BOM_DEMO` de `compose.yaml` et l'appel de `web/src/catalog.ts`).
- 2026-11-03 — Démo, choix agent à valider : l'UI a besoin des noms de pièces et des offres d'une pièce, que
  l'API `/api/v1` n'expose pas (catalogue = NLPF-67). Ajout de `GET /api/v1/demo/catalog`, monté **seulement**
  avec BOM_DEMO=1, plutôt que d'anticiper `/api/v1/parts` à la place de NLPF-67. Sans démo, l'UI affiche
  « Pièce #id » et n'ajoute que des sous-ensembles.
- 2026-11-03 — Démo, choix agent à valider : les snapshots de prix de la démo ont `scope = 'catalog'`
  (`scope_id` NULL) : `FreezePrices` ne reçoit pas l'id de la BOM et le CHECK impose `scope_id` pour `'bom'`.
  Ils figent la **dernière** cotation en base (pas celle d'avant `at`), comme `pricing.Stub`.
- 2026-11-03 — Démo : la base de dev contient maintenant des snapshots publiés (#207, #208) et donc des BOM
  (1060 « Smoke démo », 1062 « Vérif manuelle ») **non supprimables** par construction (ADR 0006), plus la BOM
  1061 de 30 031 nœuds et le brouillon 1063. Pour repartir de zéro : `docker compose down -v`.
- 2026-11-03 — T9, choix agent à valider : l'arbre est relu entièrement après chaque action
  (15,8 Mo pour 30 000 nœuds, ~0,4 s en local) ; un PATCH qui renverrait les totaux modifiés serait plus léger
  mais change le contrat T8.

## Blockers

Aucun. (Blocage T6 levé le 2026-11-03 : ADR 0008 acceptée par Arthur.presle. Blocage T3 levé le 2026-11-02 :
ADR 0006 et 0007 acceptées par Arthur.presle.)

## Verification Report(s)

<empty — appended by /spec-verify>
