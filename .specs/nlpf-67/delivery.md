# Livraison locale NLPF-67 — 2026-11-05

Branche `nlpf-67`. T1–T6 livrées ; interface et tests T7 livrés, acceptation de T7 encore bloquée
par le signal SSE réel de NLPF-68. T8 (remises BOM), T9 (seed Multipla) et T10 (validation globale)
restent ouverts. Le ticket complet n'est pas déclaré terminé.

## Responsabilités des changements de cette reprise

| Fichiers | Responsabilité |
|---|---|
| `web/src/App.tsx` | Sélection fournisseur, offres/contrats, états de chargement/erreur, relecture REST sur signal SSE, annulation des réponses périmées. |
| `web/src/index.css` | Mise en page ordinateur/mobile, focus clavier et couleurs sémantiques uniques. |
| `web/src/suppliers/Forms.tsx` | Création fournisseur, création/édition contrat, erreurs serveur ; saisie décimale conservée en texte. |
| `web/src/suppliers/HistoryDialog.tsx` | Détails de l'historique, dates/sources, accès clic/tactile/clavier et retour du focus. |
| `web/src/suppliers/format.ts` | Formatage `Intl.NumberFormat` des chaînes exactes, half-even pour les monnaies, dates UTC. |
| `web/src/suppliers/graph.ts` | Échelle graphique calculée en BigInt ; seules les coordonnées bornées deviennent des nombres. |
| `web/src/suppliers/api.ts`, `types.ts` | Transport REST, erreurs explicites et contrats JSON ; aucun calcul de montant/remise. |
| `web/e2e/fixtures.ts` | Réponses REST et EventSource contrôlés ; aucune prétention de flux serveur réel. |
| `web/e2e/suppliers.spec.ts` | Neuf scénarios exécutés sur six projets, dont précision >2^53, erreurs, isolation, clavier/tactile, reconnexion et formulaires. |
| `web/playwright.config.ts` | Chromium/Firefox/WebKit, ordinateur et petit écran, zéro retry, traces d'échec. |
| `web/package.json`, `package-lock.json`, `.gitignore` | Playwright 1.64.1 approuvé par ADR 0010, commande de test, artefacts exclus de Git. |
| `.github/workflows/ci.yml` | Ajout des mêmes tests navigateur et de leurs artefacts aux contrôles existants. |
| `README.md`, `docs/workflow.md`, `docs/api-catalogue-fournisseurs.md` | Démarrage, commandes de test, API et limites d'intégration documentées. |
| `go.mod`, `Dockerfile` | Correctif Go 1.27.3 pour GO-2026-6809, sans nouvelle dépendance. |
| `.specs/nlpf-67/{spec,state,delivery,integration-nlpf-69}.md` | État réel, preuves et impact des dépendances ; tâches incomplètes conservées ouvertes. |

Les services, adapters et tests T1–T6 sont décrits dans le journal append-only de `state.md`.

## Vérifications exécutées

| Contrôle | Résultat |
|---|---|
| `make lint` | PASS : go vet, golangci-lint (0 issue), oxlint. |
| `make test` avec `CI=true` et PostgreSQL 18 dédié | PASS ; paquets catalogstore, supplierstore, pricingstore, HTTP et migrations réellement exécutés. |
| `go test -race -count=1 ./...`, même base | PASS ; pas de tests PostgreSQL sautés faute de base. |
| goose up, reset, up | PASS, exclusivement dans la nouvelle base jetable `stp-nlpf67-resume-test`. |
| `go tool sqlc generate` | PASS : empreintes SHA256 identiques des neuf fichiers Go générés. |
| `go mod tidy -diff` | PASS, aucun changement requis. |
| `npm ci` puis build Node 22.23.4 | PASS ; installation du npm système/Node 18 initialement insuffisante, corrigée avec Node compatible. |
| `make sec` | PASS : aucune vulnérabilité après Go 1.27.3 ; audit npm production à zéro. |
| Tests navigateur Playwright 1.64.1 | PASS : **54/54**, zéro échec, zéro skip, zéro retry, 31,0 s. |
| Stack Docker Compose isolée | PASS : images construites, db/back/web/priceapi healthy, migrate Exited (0), `/`, `/healthz` et `/api/v1/suppliers` 200 via nginx, santé priceapi 200. |
| Démarrage du binaire serveur | PASS : `/healthz` 200, `/api/v1/suppliers` 200 `[]`, base jetable, serveur arrêté après contrôle. |

Le premier scan de Go 1.27.2 a échoué sur GO-2026-6809 (`crypto/x509`, correctif 1.27.3).
Le compilateur temporaire 1.27.3 et le Dockerfile sont alignés ; aucun Go/Node global ni réglage
personnel modifié. Les archives Go/Node sous `/tmp` ont été vérifiées par SHA256 officiel.

Navigateurs réels du runner : Chromium **156.0.8078.80**, Firefox **157.0**, WebKit **27.2**.
Playwright s'exécute dans `mcr.microsoft.com/playwright:v1.64.1-noble` (Node 26.13.0), réseau
désactivé, `web` monté en lecture seule puis copié sous `/tmp`. Le système Ubuntu n'a pas reçu
de bibliothèques navigateur supplémentaires. Profils 1280×900 et 390×844 ; Firefox mobile signifie
petit viewport tactile, sans prétention de Firefox Android réel. Les captures ordinateur/mobile
ont été inspectées : contenu lisible et absence de débordement horizontal.

Artefacts locaux temporaires : `/tmp/stp-browser-artifacts/t7-final/results.json` et `test-results/`.
Leur persistance entre sessions n'est pas garantie ; le résultat et les versions sont consignés ici.

## Pannes reproduites et corrigées

- Une erreur 503 devenait invisible en recliquant le fournisseur déjà sélectionné : l'effet ne
  rechargeait rien mais le clic effaçait l'erreur. La sélection identique conserve désormais son
  état. Le scénario « erreur REST explicitement visible sans prix fabriqué » passe sur six profils.
- Sur Chromium mobile, fermeture puis réouverture au clavier du dialogue d'historique échouait.
  La fermeture passe par l'état React, évite l'événement natif de fermeture différé et rétablit
  le focus. Le scénario clavier/clic/tactile inchangé passe sur les six profils.
- Prix absent : le test vérifie une erreur visible et aucun prix zéro. Une réponse tardive du
  fournisseur précédent est annulée et ne remplace pas la sélection courante.

## Contrôles et fonctions encore indisponibles

- SSE serveur réel, horloge simulée, ingestion/conversion et générateur d'historique NLPF-68.
- Calcul BOM avec contrats et seed idempotent Fiat Multipla de ~30 000 nœuds ouvert dans le vrai
  éditeur. Voir `integration-nlpf-69.md` pour les conflits avant adoption de cette branche.
- Safari réel macOS/iOS et Chrome/Edge installés chez l'utilisateur : NOT RUN. WebKit et les
  profils tactiles ne sont pas une validation sur ces appareils.
- CI distante sur cette révision : NOT RUN. Le smoke Docker Compose local passe sur une base
  dédiée neuve ; aucun push, aucune PR et aucune fusion. La définition de terminé du projet n'est donc pas remplie.
- Validation globale des anciennes specs : deux incohérences préexistantes
  `docker-compose-stack` et `nlpf-70` (spec draft/state done), laissées intactes.

Les données de l'équipe et les snapshots publiés n'ont pas été touchés. La base de test n'utilise
que des identifiants de développement jetables et des transactions de tests.
