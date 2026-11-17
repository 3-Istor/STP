# Refonte 3D — progression et reprise

Mise à jour : 2026-11-17. Branche active : `feat/3d-pro-v2`.
Base : `010d4d9e54d0990cbfe0f1368341ae386034107d` (`origin/v2/implementation`).
Étape active : liaison des occurrences par manifestes frontend, autorisée par l'utilisateur
(option 1 de l'ADR 0041, avec les sept conditions explicites de la conversation).
Les statuts historiques ci-dessous restent un journal ; l'attente d'approbation est levée.
L'ADR elle-même reste au statut documentaire Proposé : l'agent ne remplit pas le nom du décideur.

## Première action à la reprise

1. Vérifier `feat/3d-pro-v2`, l'état Git et les derniers points de sauvegarde.
2. Reprendre le résolveur et le chargeur de manifestes : IDs d'occurrences, variantes, placements
   connus, invalidation après modification, sélection exacte, classification thermique/électrique.
3. Garder séparées source du manifeste, validation pure et rendu Three.js ; aucune API Go ni table
   PostgreSQL modifiée. L'interface de source permettra une autre persistance ultérieure.
4. Continuer l'acquisition de vrais modèles des bons véhicules ; aucune correspondance par nom,
   ni géométrie ou emplacement supposé présentés comme vérifiés.

## Point de sauvegarde — changement de branche

- Autorisation : créer `feat/3d-pro-v2` directement depuis le commit V2, sans modifier les autres branches.
- Six fichiers (et non cinq) détectés, sauvegardés dans
  `/tmp/stp-3d-audit-backup-20261117T171500Z/audit-files.tar.gz` ; contrôles dans
  `docs/3d/audit-backup-checksums.json` (chemins originaux).
- SHA-256 des six fichiers comparés à l'archive après `git switch` : tous identiques.
- `git show-ref` comparé avant/après : toutes les références existantes identiques ; seule la nouvelle
  branche a été ajoutée. La création a réussi au second essai, après expiration de la première revue
  automatique de permission ; aucune opération Git n'avait eu lieu lors de ce premier essai.
- Moteur, intégration React/BOM et fixtures Multipla/Valhalla présents et lus au commit demandé.
- Outils d'inventaire déplacés de `scripts/` vers `docs/3d/tools/` avec adaptation de la racine et des
  commandes ; aucun script existant du harnais n'a été écrasé. Audit initial conservé dans l'archive.
- Les résultats de la première session plus bas restent un historique ; ils ne décrivent pas les
  nouveaux essais de validation tant que ceux-ci ne sont pas consignés.

## Travail réalisé et preuves

| Travail | Fichiers / sources | Vérification | Statut |
|---|---|---|---|
| Intégrité et base Git | HEAD, branches, merge-base, ls-remote | Branche propre initialement ; HEAD identique à main ; 101 commits V2 absents | Corrigé par nouvelle branche autorisée |
| Analyse intégrale des deux ZIP | Extraction `/tmp/stp-3d-audit/` ; SHA dans l'audit/manifeste | 9 + 436 entrées, pas d'assets automobiles binaires | Réalisé |
| Alignement des inventaires | CSV des ZIP, `internal/sampledata/{multipla,valhalla,common}.go` des deux bases | 220 + 153 + 45 = 418 clés, aucune différence | Réalisé |
| Analyse moteur V2 | `git show origin/v2/implementation:...` | Défauts occurrence, clé de scène, quantité, géométrie et éclaté consignés dans AUDIT | Réalisé par lecture statique |
| Inspection visuelle historique | 3 captures `docs/v2/captures/lot-d` de V2 | Images extraites et vues ; pas des captures nouvelles | Réalisé, limite explicitée |
| Recherche supplémentaire | Sources Aston Martin, Khronos, Commons, sites de modèles | Architecture hybride confirmée ; piste CC Attribution non téléchargée | Partiel : assets exacts manquants |
| Récupération des 12 photos | Script fourni, inspecté puis exécuté | 0/12 ; DNS dans sandbox, métadonnées numériques puis HTTP 429 hors sandbox | Échec documenté |
| Manifeste des 418 types | `docs/3d/assets-manifest.json` | Aucun ID runtime inventé, aucune géométrie déclarée fiable | Créé |
| Contrôle reproductible | `docs/3d/tools/audit_3d_assets.py`, `docs/3d/tools/test_audit_3d_assets.py` | Intégrité du seed et refus des fausses déclarations | Résultats ci-dessous |
| Plan et points de reprise | `3D_PLAN.md`, ce fichier, `docs/3d/AUDIT.md` | Étapes, dépendances, preuves et première action précisées | Créés |

## Fichiers de cette unité

- `3D_PLAN.md` : séquence de livraison et critères, proposition de correction de base.
- `3D_PROGRESS.md` : état, preuves, commandes et prochaine action.
- `docs/3d/AUDIT.md` : diagnostic du code, archives, ressources, approbations et limites.
- `docs/3d/assets-manifest.json` : inventaire de recherche ; 418 géométries `unavailable`, licence,
  provenance, preuve et placement non résolus ; deux représentations historiques séparées.
- `docs/3d/tools/audit_3d_assets.py` : contrôle sans DB de l'alignement des clés et des affirmations de couverture.
- `docs/3d/tools/test_audit_3d_assets.py` : cas valides et cas d'identité/couverture incorrectes.

Aucun fichier produit, dépendance, API, migration, ADR acceptée ou snapshot n'a été modifié.
Les fichiers sont sauvegardés dans le dépôt et dans l’archive vérifiée. Aucun push, aucune PR,
aucune fusion effectués. Les commits de la nouvelle branche sont consignés dans les points de sauvegarde.

## Commandes et résultats

- `git ls-remote --heads origin 'v2*' 'feat/3d-pro'` : réussi avec accès réseau hors sandbox ;
  confirme les commits consignés plus haut. Essai sandbox : résolution DNS impossible.
- `python3 docs/3d/tools/audit_3d_assets.py` : réussi, 418 clés alignées avec le seed ; groupes
  `commun: 45, multipla: 220, valhalla: 153`, géométries `unavailable: 418`.
- `python3 -m unittest discover -s docs/3d/tools -p 'test_audit_3d_assets.py' -v` : réussi,
  2 tests, dont une table de 11 cas incorrects (doublon, clé absente/inconnue, mauvais véhicule,
  statut inventé, asset incohérent/absent/sans licence, exactitude ou placement sans preuve).
- `git diff --check` : réussi ; les nouveaux fichiers sont encore non suivis, donc ce contrôle Git
  seul ne constitue pas une revue de leur contenu.
- `make test` : échec immédiat, sortie exacte :

  ```text
  go test ./...
  make: go: No such file or directory
  make: *** [Makefile:27: test] Error 127
  ```

- `make lint` : essaie d'abord `npm ci` (node_modules absent), puis échoue avant Go/front lint :

  ```text
  npm ERR! code EAI_AGAIN
  npm ERR! syscall getaddrinfo
  npm ERR! request to https://registry.npmjs.org/vite/-/vite-8.3.8.tgz failed, reason: getaddrinfo EAI_AGAIN registry.npmjs.org
  make: *** [Makefile:65: web/node_modules] Error 1
  ```

  Node présent : `v18.19.1`, npm `9.2.0`. Avertissements `EBADENGINE` sur Vite, plugin React,
  Playwright et oxlint. Ne pas contourner les exigences en changeant les versions verrouillées.
- Photos : script original exécuté, puis correctif temporaire
  `html.unescape(v.get('value',''))` → `html.unescape(str(v.get('value','')))`.
  Le dernier essai (`/tmp/stp-3d-audit/photos-download-fixed.log`) finit par
  `0/12 images téléchargées et revérifiées`, exit 2, HTTP 429 sur les 12 fichiers.
  Aucun fichier image ni crédit de téléchargement réussi à importer. Ne pas répéter les appels
  immédiatement ; conserver les contrôles de licence et traiter les métadonnées non textuelles.
- Build, tests unitaires V2, Playwright, CI, mesures GPU/FPS : non exécutés ; base produit et outils
  non prêts. Chrome système et un cache Playwright existent ; aucune validation WebGL actuelle.

## Couverture et limites

418 types de catalogue inventoriés ; 0 géométrie de pièce fiable acquise ; 0 placement constructeur
validé. Aucun nombre d'occurrences réellement représentées mesuré sur une BOM complète en exécution.
Les 56/52 objets des fixtures démo V2 sont des sous-ensembles, pas une preuve de couverture complète.
Les 13 éléments non géométriques du CSV restent classés tels quels, sans inventer de mesh.

Risques encore ouverts : licence et découpage des modèles candidats, architecture d'identité stable,
ancien mesh après remplacement, occurrence confondue avec son sous-ensemble, emplacements de quantités
supplémentaires inconnus, recherche masquant une sélection, focus ne rouvrant pas l'onglet Détail.
Rien de cela n'est annoncé comme corrigé. La définition de terminé du projet n'est pas atteinte.

## Point de sauvegarde — audit récupéré sur V2

- Nouvelle branche vérifiée ; règles V2 et harnais relus. `go run ./cmd/risk -base 010d4d9` : niveau 0
  pour cette unité documentaire (les outils de l'inventaire accompagnent ses données dans `docs/3d/tools`).
- Node `24.22.3` et Go `1.27.4` installés dans `/tmp/stp-3d-toolchain`, archives officielles vérifiées
  par SHA-256. PATH de travail : `/tmp/stp-3d-toolchain/node-v24.22.3-linux-x64/bin:/tmp/stp-3d-toolchain/go/bin`.
  Caches Go : `GOMODCACHE=.../gomod`, `GOCACHE=.../gocache`, `GOPATH=.../gopath` sous ce même dossier.
- `npm ci` réussi sans changement des fichiers de dépendances ; audit npm signale 5 vulnérabilités
  élevées préexistantes. Ne pas appliquer `npm audit fix --force` sans décision de dépendances.
- Tests unitaires web de base : 7 fichiers réussis. Lint réussi avec 4 avertissements existants
  (`useLoad.ts`, `Ideas.tsx`, `Comparison.tsx`). Build TypeScript + Vite réussi ; chunk 3D différé ~677 kB.
- `make verify` rapide : réussi ; gitleaks absent et indiqué `not_run` par le harnais.
- `make test` : domaines purs, architecture et routes passent ; intégration échoue sur la base locale
  non migrée à V2 : `ERROR: column "cost_basis" does not exist (SQLSTATE 42703)` et
  `ERROR: column "tooling_amount" does not exist (SQLSTATE 42703)`, puis `make: *** [Makefile:27: test] Error 1`.
  Les tests utilisent des transactions annulées ; aucune migration n'a été appliquée à cette base.
  Prochaine vérification backend : base de test isolée avec les migrations V2, sans toucher aux données utilisateur.
- CarConcept téléchargé au chemin déjà ignoré `web/public/models/CarConcept.glb` (~11,2 MiB) pour
  exercer le chargeur existant. Ce modèle CC BY 4.0 reste une approximation sans lien OEM ; aucun
  des 418 types n'est promu en géométrie fiable.
- Contrôle Python et ses 2 tests (11 sous-cas invalides) réussissent après déplacement.
- Ce point est un commit de reprise documentaire ; les étapes visuelles et fonctionnelles restent ouvertes.

## Point de sauvegarde — transitions et matériaux (étapes 2/3 partielles)

- Commit documentaire précédent : `0048639`.
- `viewer.ts` : transitions des pièces depuis la pose affichée, inversion sans saut, arrivée copiée
  exactement à la pose cible, animation de caméra annulée par une navigation manuelle, mouvement
  réduit respecté, cadrage rapproché et zoom borné. Le rendu cesse au repos ; diagnostics disponibles
  dans le point d'accès de développement existant.
- `motion.ts` et son test : courbe d'animation commune, bornes exactes ; le test a échoué avant ajout
  du module, puis passe. Les tests existants de mapping et d'éclatement restent présents et passent.
- `types.ts`, `colors.ts`/test, `Vehicle3DView.tsx`, `Structurer.tsx`, `demo/Demo.tsx` : mode Matériaux
  sans coloration budget implicite ; modes Budget et Système conservés et vérifiés.
- `simpleModel.ts` : couche de vernis de la carrosserie dans le schéma généré.
- Nouveau `web/e2e/vehicle3d-viewer.spec.ts` : deux fixtures, 0→100→0 exact, inversion, sélection,
  repos sans boucle permanente, préférence de mouvement réduit, GLB absent/invalide.
- Chromium desktop : **5 tests passent**. Premier essai à 5 s trop court pendant l'initialisation
  WebGL logicielle ; délai de chargement des nouveaux tests porté à 30 s, sans skip ni retrait de test.
  Durées mesurées par parcours 13–27 s, pas une mesure du seul temps de chargement ni des FPS.
- Quatre captures nouvelles inspectées visuellement : `docs/3d/captures/{multipla,valhalla}-{assemble,eclate}.png`.
  Constat : schémas génériques, silhouette non fidèle, positions Valhalla estimées et collisions encore
  possibles. Le gain porte sur les matériaux et interactions ; la qualité automobile professionnelle
  de l'étape 2 n'est PAS validée, ni la fidélité industrielle de l'étape 3.
- Tests unitaires web : 8 fichiers passent ; lint et build passent avec avertissements préexistants.
- `make lint` : réussi avec les outils temporaires ; `make test` sur PostgreSQL temporaire migré :
  réussi, intégration comprise (voir `/tmp/stp-3d-toolchain/go-test-v2.log`).
- Cas de panne couvert : retour à 0 après inversion pendant le déplacement ; pose finale strictement
  identique. GLB absent/invalide produit un message et un repli explicitement schématique.
- Prochaine unité indépendante : affichage explicite des pièces sans modèle, outils de cadrage/focus
  et transparence, puis proposition d'ADR pour les liaisons exactes par occurrence.

## Point de sauvegarde — sélection explicite et commandes (étapes 2/3/4 partielles)

- Commit précédent : `48a34a4` (transitions et matériaux).
- Sauvegarde originale des six fichiers désormais aussi dans `docs/3d/audit-initial-backup.tar.gz`,
  vérifiable par `audit-backup-checksums.json` : elle survivra à la suppression des fichiers `/tmp`.
- `selection.ts`/test et `Vehicle3DView.tsx` : les pièces physiques sans mesh n'empruntent plus la
  sélection de leur parent ; commandes désactivées et état explicite. Les groupes ne ciblent que
  les géométries présentes dans le modèle effectivement chargé. La racine cible ses descendants.
- `viewer.ts` et contrôles React : cadrage, focus, transparence de la carrosserie, reset synchronisé
  avec le curseur ; garde contre un focus de boîte vide. Recréation du moteur après erreur WebGL
  via « Réessayer ». Les sous-ensembles gardent leurs limites schématiques.
- `vehicle3dNodes.ts`/test : remplacer `part_id` invalide la clé ; changer uniquement `offer_id` ne
  reconstruit pas la géométrie. Les placements et quantités physiques ne sont pas encore résolus.
- `BomWorkspace.tsx` : sélectionner un nœud ouvre Détail, y compris depuis l'onglet Réglages.
- `simpleModel.ts` : vernis limité à la carrosserie, peinture bleue illustrative pour le contraste ;
  les autres matériaux gardent MeshStandardMaterial. Couleur de démonstration, pas teinte constructeur.
- `web/e2e/bom-cost-v2.spec.ts` : le parcours existant est conservé et renforcé pour le contrat
  demandé (pièce sans modèle : sélection vide + message + focus désactivé, groupes : descendants),
  puis ouverture du panneau depuis Réglages. Aucun test n'a été retiré ou sauté pour passer.
- Tests unitaires web : 9 fichiers passent ; cas de remplacement physique rouge avant correction.
  Cas « sous-ensemble absent du GLB » rouge avant correction puis vert.
- Chromium desktop : **57 parcours passent**, dont 8 nouveaux parcours du visualiseur. Parcours
  intégré Arbre + 3D vérifié aussi sur Chromium mobile 390×844 (1 PASS).
- Relecture `relecteur-code`, contexte neuf, 3 tours : 1 Important de focus absent corrigé avec
  test ; 1 Important sur coordonnées de test mobile corrigé en mesurant après le défilement ;
  dernier tour « Aucun constat ». L'approbation humaine reste distincte de cette revue.
- Moulinette `make verify LEVEL=pr` verte : tidy, sqlc, lint, Go -race, govulncheck, tests/build web,
  audit npm de production (0 vulnérabilité). Gitleaks, osv-scanner, actionlint absents = not_run.
  Voir `docs/3d/verification-pr.md` pour le libellé PostgreSQL du harnais et la comparaison de risque.
- Capture réelle de CarConcept : rendu assemblé/éclaté pour les deux fixtures, sans erreur JS.
  `carconcept-validation.json` : 322 draw calls, 639587 triangles par image, 108 géométries,
  17 textures ; chargements locaux mesurés 7952 / 6225 ms dans Chromium de cet environnement.
  C'est le même véhicule conceptuel dans les deux cas, pas la Multipla ni la Valhalla demandées.
- Schémas : Multipla 201 draw calls / 116894 triangles ; Valhalla 171 / 90902, après chargement.
  FPS pendant interaction non mesurés ; aucun objectif de fluidité haut de gamme validé.
- Captures dans `docs/3d/captures/`, inspection visuelle : meilleur contraste, commandes lisibles ;
  silhouettes et mécanique encore génériques, éclaté CarConcept avec chevauchements visibles.
- Étapes 2/3 non terminées (assets fidèles et placement manquants). Étape 4 : absence honnête et
  sélection des sous-ensembles réalisées ; liaison exacte aux pièces en attente de décision.
  Étape 5 non terminée ; changements de quantités/instances et substitutions géométriques à réaliser.
  Étape 6 : validation locale partielle ; CI distante, Firefox/WebKit et vraie BOM complète non validés.
- Prochaine action : présenter le contrat explicite de liaison 3D des occurrences dans une ADR
  proposée, sans changer API, schéma, dépendances ou snapshots avant l'accord requis.

## Point de reprise — décision de liaison fine

- Commits locaux : `0048639` audit ; `48a34a4` transitions/matériaux ; `ba468c4` absence/commandes.
- ADR 0041 créée au statut **Proposé** : option 1 recommandée (manifestes frontend explicites),
  alternatives avec persistance catalogue/API ou maintien des seuls schémas. Aucune implémentation
  de ce nouveau contrat, aucune acceptation d'ADR par l'agent.
- Autorisation requise par `docs/agents/travail-avec-agents.md` §6–8 et la procédure
  `.claude/skills/rediger-adr/SKILL.md` : arrêter le travail qui dépend de la décision et présenter les options.
- Avant cette attente : toutes les modifications frontend de cette livraison sont sauvegardées,
  testées et relues. L'acceptation globale des étapes 2–6 reste ouverte, pas déclarée terminée.

## Vérification finale et nettoyage de cette session

- Dernière correction de test confirmée sur desktop : 1 PASS ; sur mobile : 1 PASS, sans modification
  de l'assertion de sélection ni du scénario utilisateur. Suite desktop complète précédente : 57 PASS.
- Relecteur d'architecture sur l'ADR 0041 proposée : « Aucun constat » ; cela ne vaut pas acceptation.
- Nouvelle comparaison des références Git : toutes les références préexistantes sont identiques
  à la sauvegarde d'avant changement de branche. Archive des six fichiers originaux revérifiée par SHA-256.
- PostgreSQL temporaire `stp-3d-pro-v2-testdb` arrêté et supprimé automatiquement (sans volume).
  Vite de validation sur 41768 arrêté. Aucun serveur de validation n'est laissé actif.
- Pour reproduire l'intégration : recréer le conteneur temporaire avec PostgreSQL 18, port 55439,
  utilisateur `stp3d`, mot de passe de test `stp3d-test`, base `stp3d`, appliquer les migrations existantes,
  puis `make test DATABASE_URL='postgres://stp3d:stp3d-test@127.0.0.1:55439/stp3d?sslmode=disable'`.
- Prochaine action dépendante inchangée : validation humaine de l'option 1 de l'ADR 0041.

## Correction des preuves visuelles

- Après inspection, une image CarConcept « éclatée » montrait encore le début de transition.
  Capture refaite après comparaison à la pose assemblée ET attente de fin de l'animation, puis
  inspectée : pièces effectivement séparées ; chevauchements toujours signalés, pas d'exactitude OEM.
- Méthode sauvegardée dans `docs/3d/tools/capture_carconcept.mjs` (dépendance Playwright existante,
  Vite dev et `make models` requis). Contrôle de syntaxe Node réussi. Le script de capture temporaire
  utilisé pour cette recapture applique la même attente de pose finale.
- Dernières mesures CarConcept dans le JSON : 13158 / 20938 ms dans cet environnement ; ce sont
  des temps incluant initialisation, chargement et compilation WebGL, pas des garanties de performance.
- Vite arrêté après recapture. Code produit et résultats fonctionnels inchangés.

## Accord utilisateur reçu — ADR 0041 option 1

L'utilisateur a explicitement validé les manifestes frontend et autorisé leur implémentation automatique.
Conditions : occurrence exacte même pour une pièce répétée, cohérence après édition, remplacement
physique distinct de l'offre, absence explicite, source extensible vers PostgreSQL, backend préservé.
Cette validation constitue l'autorisation requise ; aucune nouvelle demande n'est nécessaire pour ce périmètre.
La ligne de statut et les Décideurs de l'ADR sont laissés à l'humain selon les règles du projet.
Plan d'incréments : contrat/résolveur et tests ; chargeur/rendu/import ; instances/variantes/modes ;
validation fonctionnelle et visuelle sur les assets réellement accessibles.

## Incrément — contrat des manifestes et recherche des véritables assets

- `manifest.ts` : validation pure et résolution sur l'arbre courant, sans réseau ni Three.js.
  Identités exactes, variantes par nœud/parent/pièce, produit décimal exact des quantités, positions
  connues seulement, déplacement hiérarchique explicite, portée et révision de snapshot contrôlées.
- `manifest.test.ts` : répétitions de pièce, variantes, offre seule, ajout/suppression/déplacement,
  positions insuffisantes, fraction/héritage, IDs imprécis, preuves/chemins invalides, arbre de 30000 nœuds.
- Tests unitaires : 10 fichiers passent ; build TypeScript/Vite passe. Le module n'est pas encore
  raccordé au viewer : ce point sauvegarde le contrat et ses preuves avant chargement/rendu.
- `MANIFEST_FORMAT.md` décrit les données ; `ASSETS_NEEDED.md` précise les modèles et organes manquants.
- API publique Sketchfab : quatre candidats Multipla/Valhalla avec CC BY 4.0 déclaré. Le téléchargement
  officiel exige une authentification (401) ; aucun fichier obtenu, aucun contournement du viewer.
  La provenance et le découpage restent à vérifier ; la fidélité visuelle n'est pas annoncée acquise.
- Accord ADR 0041 reçu et consigné ; statut documentaire laissé à l'humain. Aucun backend modifié.
- Prochaine action : source statique/import JSON, chargeur GLB avec contrôle d'empreinte, réutilisation
  et libération des ressources ; puis liaison au mode Composants, variantes et classification hybride.
