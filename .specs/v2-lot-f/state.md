---
id: v2-lot-f
status: review
branch: v2-lot-f
current_task: null
started: 2026-11-17
updated: 2026-11-17
---

# État — V2 lot F (intégration front)

## Progress

Partie 1 terminée (T1 à T8), en attente de relecture. Partie 2 (versions, contrôles de publication,
analyses, profils : lots B et E) : après leur fusion.

## Task Log

- 2026-11-17 T1 — spec et état créés après lecture d'AGENTS.md, `.specs/AGENTS.md`, du plan, des maquettes
  I4-1/2/3/4, des états des lots A, C, D, des ADR 0031, 0034, 0035 et de `web/src`. (4b16583)
- 2026-11-17 T2 — back : `BudgetStatus.Percent`, budgets évalués par `Service.Totals` et renvoyés dans
  `GET /tree` (`budgets`), `percent` dans `GET /budgets`. Constat qui motive le champ dans `/tree` : sur la base
  seedée en prix en direct, `/budgets` et `/tree` lus à 1 s d'écart donnaient 25 764,89 € et 26 115,51 € pour la
  racine (l'horloge simulée avance entre deux requêtes). Mutation `RoundBank` → `Round` : test rouge. (7a43558)
- 2026-11-17 T3 — colonne Budget %, budget global et barre sous le total, « Hors budget » cliquable, barres
  « % du budget » par sous-système (`bom/budget.ts` : mise en forme seulement). (34b44bd)
- 2026-11-17 T4 — `Vehicle3D` dans Structurer (`Vehicle3DPane` : Rendu, Colorer, Éclatement), `Vehicle3DSlot`
  supprimé, `bom/vehicle3dNodes.ts` (statut budget + clé de scène). Vérifié en vrai : double-clic sur la roue →
  focus `wheel_fl`, `?node=` et arbre suivent. Puis frontière d'erreur : sans WebGL, `WebGLRenderer` levait dans un
  effet et toute l'application disparaissait. (fa8ddf3, 3099afa)
- 2026-11-17 T5 — méthode et date de calcul dans la Synthèse ; réglages Achat / Rendu / Complet, Taux budget /
  Dernier taux, volume programme ; titre du total selon la base. Vérifié : Complet → Achat → « Recalcul en cours… »
  puis « Coût d'achat par véhicule » ; méthode remise à full/budget/60 000 ensuite. (d24f5bb)
- 2026-11-17 T6 — `bom/CostBreakdown.tsx` dans le Détail (brouillon et snapshot), budget du nœud dans les faits ;
  Chiffrer renvoie vers le Détail. (f516654)
- 2026-11-17 T7 — `web/e2e/bom-cost-v2.spec.ts` (6 tests × 2 projets). Premier jet instable sous charge
  (deux montages 3D > 30 s) : un seul chargement, délai 90 s pour ce test. (409baf2, 18dc0ca)
- 2026-11-17 T8 — vérification réelle et captures (rapport ci-dessous).

## Decisions

- D1 (seul, à valider) : budgets renvoyés dans `GET /tree` et `percent` calculé par le back (coût ÷ budget × 100,
  half-even à 1 décimale, null si indisponible ou budget nul). Champs ajoutés, rien retiré ; l'état reste décidé
  sur les montants exacts (seuil orange ≥ 90 %, `pricing.EvaluateBudget`, ADR 0018). Le front n'en fait que la
  mise en forme. Alternative écartée : % calculé côté front (règle de calcul côté écran) ou `/budgets` lu à part
  (chiffres incohérents en prix en direct).
- D2 (seul, à valider) : dans la 3D, un sous-ensemble sans budget propre est gris (« sans budget ou
  indisponible »), il n'hérite pas du statut de son système. Sur la Multipla seedée, seuls Porte avant gauche,
  Porte avant droite et Capot ont un budget au niveau 2 : le reste du véhicule est gris. Hériter du statut du
  système donnerait une voiture « colorée » comme la maquette mais affirmerait un dépassement non calculé.
- D3 : snapshot publié → aucun budget (non figés par le lot A, à voir avec le lot B) : colonne « — » avec
  infobulle, « n/d » dans la Synthèse, 3D colorée par système avec l'avis du lot D.
- D4 : « Pourquoi ce coût ? » montre « absente » pour une composante attendue par la base de la méthode
  (ADR 0031 : achat = prix ; rendu = + remise, douane, transport, emballage ; complet = + outillage) sans ligne
  dans la réponse. Le change n'est pas attendu (ligne présente seulement s'il y a conversion).
- D5 : rendu, couleur et éclatement de la 3D sont un état local (pas dans l'URL) ; Simple par défaut (ADR 0035).
  La scène n'est reconstruite que si structure ou statuts changent (`vehicle3dKey`), pas à chaque relecture.
- D6 : pourcentages affichés à 1 décimale (« 97,2 % ») et non arrondis à l'entier comme la maquette, pour qu'un
  89,6 % vert ne s'affiche pas « 90 % ».
- D7 : sous 760 px, « Arbre + 3D » empile l'arbre au-dessus de la 3D.

## Blockers

Aucun.

## Verification Report(s)

### 2026-11-17 — T8 (partie 1)

Environnement : base `stp_lot_f` (goose up → 00012, `server seed` demo-v4), API de prix `:8096`, serveur
`:8086` (`PRICE_API_URL=http://127.0.0.1:8096`), Vite `:5186` (`STP_API_PROXY=http://localhost:8086`),
`make models` (gltfpack absent : modèle non compressé). Un snapshot #1 de la Multipla a été publié sur cette base
de dev pour vérifier la cascade d'un snapshot. Tests Go d'intégration sur `stp_lot_f_test`.

| Vérification | Résultat |
|---|---|
| `go vet ./...` | PASS |
| `go tool golangci-lint run ./...` | PASS — `0 issues.` |
| `go test -race -count=1 ./...` (avec `DATABASE_URL`) | PASS — 18 paquets `ok` |
| `go mod tidy -diff`, `go tool sqlc generate` sans écart | PASS |
| `cd web && npm ci` / `npm audit --omit=dev --audit-level=high` | PASS — 0 vulnérabilité |
| `npm run lint` (oxlint) | PASS (code de sortie 0) |
| `npx tsc -b` | PASS |
| `npx vite build` | PASS — `three` seulement dans `Vehicle3DView-*.js` (677 kB, 172 kB gzip) ; entrée `index-*.js` 361 kB sans `three`. Avertissement Vite > 500 kB sur ce chunk chargé à la demande (déjà signalé par le lot D, non traité) |
| `npm run test:unit` | PASS — 22/22 |
| Playwright `chromium-desktop` + `chromium-mobile` (Chrome système, SwiftShader, config locale non commitée) | PASS — 86/86, deux passes complètes ; test 3D répété 4× : 8/8 |
| Playwright `firefox-*`, `webkit-*` | NON EXÉCUTÉ (navigateurs absents de la machine ; le test 3D se saute avec raison si WebGL2 manque) |
| CI GitHub | NON EXÉCUTÉE (rien n'est poussé) |

Captures relues (1440×900) dans `docs/v2/captures/lot-f/` : 01 arbre, 02 arbre + 3D réaliste colorée par budget,
03 focus roue, 04 réglages de calcul, 05 « Pourquoi ce coût ? » (brouillon), en clair et sombre ; 06 cascade d'un
snapshot (clair). Prix en direct : les chiffres diffèrent d'une capture à l'autre (horloge simulée).

Non fait / non vérifié :
- La cascade est une requête séparée de l'arbre : en prix en direct, son total peut différer de la colonne Coût
  d'une cotation (constaté identique sur les captures, non garanti).
- Libellés de la cascade tels que l'API les construit (décimales brutes « 1.1458828780 »).
- La 3D reste sur fond clair en thème sombre (styles du lot D).
- Profil de véhicule : emplacement seulement (lot E, partie 2).
