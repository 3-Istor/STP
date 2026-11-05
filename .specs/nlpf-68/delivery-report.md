# NLPF-68 — livraison autonome et vérifications

2026-11-05, branche `nlpf-68`. ADR 0011–0016 acceptées explicitement par Joe.
T1–T9 terminées pour les tranches autonomes convenues (ADR 0016). T10 reste ouvert :
intégration BOM coordonnée et CI distante non réalisées. Aucun push, merge ni PR.

## Comportement livré

Source HTTP déterministe, cinq requêtes ; 365 jours initiaux et ticks persistants atomiques,
reprise sans rattrapage ; prix/FX/historiques réels et repli complet daté ; snapshots quatre
périmètres, couverture et fermeture au commit ; conversion directe/inverse rationnelle exacte ;
évaluateurs purs de budgets et volume de référence ; SSE et écran fournisseurs actualisé.
La vraie BOM, ses budgets persistés, ses publications et le seed Multipla ne sont pas intégrés.

## Responsabilités des fichiers

| Fichiers principaux | Responsabilité |
|---|---|
| `internal/pricesim/`, `pricing/source.go`, `priceclient/`, `priceapi/quotes.go` | Génération économique pure, contrats/validation et transport HTTP externe |
| `pricing/runtime.go`, `pricingstore/runtime.go`, migration 00003 | Historique, manifeste, curseur, bail et lot validé atomiquement ; pas de réseau sous transaction longue |
| `pricing/reads.go`, `pricingstore/reads.go`, `httpapi/pricing.go` | Dernières cotations à l’horloge validée, historiques et repli sur un snapshot complet |
| `pricing/snapshot.go`, `pricingstore/snapshot.go`, `httpapi/pricing_snapshots.go`, migration 00004 | Couverture, gel atomique, lecture par IDs et fermeture SQL sans changement des faits legacy |
| `money/exact.go`, `pricing/conversion.go`, `httpapi/pricing_conversion.go` | Opérations rationnelles, provenance FX et arrondi half-even à la sortie HTTP uniquement |
| `pricing/budget.go`, `production_volume.go` | Comparaisons/allocation et floor exact à coût de référence fourni ; aucun second moteur BOM |
| `httpapi/pricing_stream.go`, `cmd/server/` | Signal borné après commit, état de panne, câblage/arrêt/horloge réelle et logs structurés |
| `web/src/App.tsx`, configs Vite/Playwright, `web/e2e/`, `.github/workflows/ci.yml` | Rafraîchissement REST, prix manquant/repli, tests contrôlés et réels ; job web-live additif |
| `queries.sql` et trois dossiers `*db/` | Requêtes typées et génération sqlc cohérente avec les migrations additives |
| Tests Go et `pricingstore/{browser,performance}_integration_test.go` | Vérification métier, SQL/HTTP, concurrence, navigateurs réels et mesure bornée |

## Vérifications exécutées

- PASS : `make lint`, `make test` avec `CI=true` et PostgreSQL 18 isolé,
  `go test -race -count=1 -timeout=90s ./...`, build TypeScript/Vite.
- PASS : `make sec` — govulncheck aucune vulnérabilité, npm audit production 0 ;
  `go mod tidy -diff` vide ; sqlc stable (9 fichiers générés identiques).
- PASS : goose up/reset/up sur le seul conteneur jetable ; ancien snapshot migré sans
  changement d’ID, prix, taux ni date. Liens initiaux/savepoints acceptés, ajouts après commit,
  tentative de réouverture et transaction concurrente refusés ; XID fourni remplacé par le vrai.
- PASS : 60 parcours UI contrôlés, 31,2 s ; 12 parcours réels, 43,5 s navigateur, test Go sous
  -race réussi. Chromium 156.0.8078.80, Firefox 157.0, WebKit 27.2, ordinateur et petit écran.
  Aucun retry, skip ou scénario flaky dans ces deux exécutions navigateur.
- PASS : stack Compose distincte construite, tous services sains et migration Exited (0).
  nginx → back → source/PG ; tick réel → SSE → REST ; snapshot stable, panne → repli daté → reprise.
  SIGTERM avec SSE ouvert : serveur sort 0. Redémarrage à cadence ralentie : même tick, mêmes IDs,
  montants, recorded_at et snapshots, aucun rattrapage ni régénération.
- PASS : isolation des baux de tests corrigée après une collision entre suites simultanées ;
  attente bornée et dataset distinct. Tous les contrôles complets ont été rejoués verts ensuite.
- PASS : paire spec/state NLPF-68 validée ensemble ; FAIL global préexistant limité aux statuts
  incohérents de nlpf-70 et docker-compose-stack, laissés intacts.
- NOT RUN : Safari macOS/iOS réel, CI distante de cette révision, raccordements BOM du tableau
  suivant. Le test navigateur/mesure est opt-in dans `make test` et a été exécuté séparément.

Les artefacts de contrôle sont sous `/tmp/nlpf68-browser-final-rdb778` et
`/tmp/nlpf68-browser-live-race-l6a3KV` ; la source du scénario live est commitée.
Les essais initiaux ont révélé un sélecteur de texte de test incorrect et l’isolation des baux ;
ils ont été corrigés sans suppression de test ni désactivation d’un contrôle.

## Matrice d’acceptation

| Critère | Preuve obtenue | Reste à vérifier |
|---|---|---|
| J1 génération déterministe | Graine/ordre/batches/identités logiques, bornes exactes, HTTP cinq opérations | Aucun manque autonome |
| J2 immutabilité des cotations | Refus SQL UPDATE/DELETE/TRUNCATE et contrôles de finitude | Aucun manque autonome |
| J3 endpoints prix/FX | HTTP + PostgreSQL, fournisseur isolé, devise/source/date/ID | Aucun manque autonome |
| J4 snapshot inchangé | Liens fermés et nouvelles cotations sans effet, API/navigateurs réels | Attachement/publication du vrai brouillon |
| J5 conversion | 100 USD × 0,92 = 92 EUR, inverse/pas de pivot, somme périodique exacte 10 USD, JPY/KWD | Adaptation et agrégation du vrai moteur BOM |
| J6 365 jours/reprise | Deux lancements/concurrence, port seed annulable, redémarrage des binaires/Compose | Seed Multipla NLPF-67 sur base commune |
| J7 cadence | Horloge virtuelle 365 jours/40 min, autres vitesses ; reprise sans rattrapage | Aucun manque autonome |
| J8 actualisation | Vrais source/DB/SSE/REST/UI, trois moteurs, fournisseur/détail conservés, reconnexion | Écran BOM et ses budgets |
| J9 budgets | Seuils/zéro/taux manquant et allocations imbriquées/inverse exact testés | Persistance/API/UI et publications BOM |
| J10 66 voitures | floor exact 1 000 000/15 000=66, référence/remise gardées, zéro/overflow | Paramètres et contexte dans la vraie BOM publiée |
| C1 historique/absence | Offre/FX, bornes et futur rejeté, aucun prix zéro fabriqué | Aucun manque autonome |
| C2 couverture | Quatre périmètres, tous les manques, rollback total, port de contrôle | Refus réel du brouillon insuffisamment couvert |
| C3 panne | Repli unique complet, provenance, sans repli erreur ; outage/reprise réels | Même comportement dans le vrai moteur BOM |
| C4 terminé/compatibilité | ADR acceptées, checks locaux verts, trois moteurs | CI distante, revue humaine et dépendances ci-dessus |

## Mesure S1

Test reproductible `NLPF68_PRICING_MEASURE=true ... go test -v -count=1 -timeout=120s
-run '^TestPricingCatalogueMeasurement$' ./internal/pricing/pricingstore`.

30 000 nœuds pièce + racine, **300 offres uniques**, 109 500 prix et 365 FX initiaux.
Source HTTP réelle, PostgreSQL 18, fixture synthétique sur schéma privé, nettoyage en fin de test.

| Opération | Mesure locale observée |
|---|---:|
| Historique initial | 21,374 s |
| Tick complet | 67,756 ms |
| Lecture de 300 prix courants | 100,309 ms |
| Snapshot du périmètre BOM (300 offres dédupliquées) | 155,783 ms |
| Taille prix + FX, tables et index, après un tick | 37 920 768 octets |

Ce relevé unique n’est ni un percentile ni une garantie de production. Les métadonnées/liens
ne sont pas inclus dans cette taille. **30 000 offres uniques et recalcul BOM <1 s non mesurés** :
le vrai moteur n’est pas présent. Pas de cache/COPY/service ajouté sur une hypothèse de charge.

## Panne prévisible et test qui la détecte

Une cotation de mauvaise devise est rejetée par la validation Source ; un lot invalide ou un
échec de commit n’avance pas l’horloge. Une source ensuite injoignable donne le dernier snapshot
unique complet avec sa vraie date, ou une erreur sans repli utilisable. Les tests source/client,
transaction/rollback, REST PostgreSQL et le scénario navigateur live panne/reprise couvrent ces
protections. Sans snapshot complet, le calcul reste indisponible, jamais valorisé à zéro.

## Condition de reprise

Voir `integration-nlpf-69.md` : accord du propriétaire de NLPF-69, base/révision commune et
procédure validée pour les historiques de migration/ADR divergents. L’accord de Joe sur les
ADR 0011–0016 est acquis et ne doit pas être redemandé. Aucun ancien snapshot de l’équipe touché. Correctif final `1ed7f6d` : erreurs JSON conservées
avant ouverture SSE, test rouge puis vert et lint/test/sec/race HTTP rejoués avec succès.
