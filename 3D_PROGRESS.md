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

## Incrément — chargement des assets liés

- `manifestSource.ts` fournit l'adapter de source statique, remplaçable sans changer le résolveur.
- `boundModel.ts` contrôle SHA-256 et GLB autonome, sélectionne les sous-arbres explicites, conserve
  les transformations source et les matériaux PBR, partage géométries/textures entre occurrences,
  clone les matériaux modifiables et libère les ressources au dernier propriétaire.
- Aucun mesh n'est substitué à une ressource absente, corrompue ou ambiguë. Le diagnostic conserve
  les IDs des occurrences concernées et la couverture physique réelle.
- Tests du chargeur : occurrences répétées, transformations parentales, matériaux indépendants,
  cache et libération, empreinte incorrecte, HTTP 404, chemin de mesh absent et URI externe refusée.
  Tests unitaires : 12 fichiers PASS ; build TypeScript/Vite PASS (avertissement de taille Three.js).
- Raccordement au viewer et aux commandes en cours, validation navigateur à suivre.

## Incrément — vue liée, variantes, instances et modes de motorisation

- Mode **Composants** dans Structurer et la démo, source statique par portée ou import JSON local.
  L'import est en mémoire ; aucune écriture API, aucun enregistrement PostgreSQL ni publication.
- Sélection et picking par `node_id`/instance, groupes via leurs descendants réels. La scène courante
  masque ses anciens objets pendant un chargement ; aucun picking d'une variante périmée.
- Quantités héritées transmises par l'adapter et prises en compte par la clé du rendu lié. Les
  placements connus sont utilisés ; quantité non représentable ou placement absent = état explicite.
- Variantes choisies par occurrence/parent/pièce physique. Une variante connue remplace réellement
  la géométrie ; une inconnue retire l'ancienne. Offre seule : UUID des objets et nombre de requêtes
  GLB conservés dans le test intégré. Le backend de remplacement existant n'est pas modifié.
- Thermique/électrique/hybride : classification explicite, transparence des composants opposés et
  de la carrosserie, isolation de système. Ce sont des vues des composants disponibles, pas une
  simulation de flux, et elles ne créent aucune motorisation absente.
- Couverture affichée en occurrences physiques, placements approximatifs, liste des indisponibles,
  provenance/licences à la demande. Zéro asset chargé n'est jamais annoncé « exact ».
- Les snapshots n'ont pas de révision de manifeste publiée dans le contrat existant : rendu lié
  refusé explicitement sans révision figée ; aucune altération des données immuables.

## Validation de cet incrément et limites restantes

- `make lint` et `make test` PASS contre PostgreSQL 18 temporaire sur 55439, avec migrations
  existantes appliquées dans cette seule base. Aucun fichier Go/API/migration ou lockfile modifié.
- `make verify LEVEL=pr` vert : lint, architecture, contrat des routes, sqlc/tidy, tests Go `-race`,
  govulncheck, tests frontend, build et audit npm de production (0 vulnérabilité).
  Le harnais étiquette les tests « sans base » et l'intégration `not_run` faute de `pg_isready`
  installé ; `DATABASE_URL` était bien fourni et les suites avec base ont tourné. Ne pas transformer
  ce libellé du harnais en preuve d'un check distinct. Gitleaks, osv-scanner, actionlint : `not_run`.
  Première exécution interrompue au cours des tests race ; relance complète verte (174 s pour race).
- Après corrections de revue : 12 fichiers de tests unitaires PASS, build et oxlint PASS ;
  `make verify` rapide vert. Quatre avertissements oxlint préexistants, taille du chunk Three.js
  toujours avertie ; aucune règle abaissée, aucun test désactivé.
- Playwright : suite Chromium desktop complète **59 PASS**. Nouveaux scénarios liés desktop/mobile :
  **4 PASS**, dont arbre relu, occurrence répétée, offre seule, quantité, géométrie de remplacement,
  ancien rendu masqué pendant l'attente, variante inconnue, trois modes, picking réel et import invalide.
  La colonne Offre est masquée dans la vue combinée existante : son événement est déclenché par le
  test pour contrôler la relecture sans démonter le canvas ; ce n'est pas une validation de son accès
  tactile. Les fixtures restent synthétiques, pas une validation d'assemblage des vraies voitures.
- Deux bugs trouvés par le navigateur puis corrigés : contexte d'appel de `fetch`, nom accessible
  des filtres. L'attente du démarrage WebGL et le point de picking mobile ont été rendus explicites,
  sans retirer les assertions. Captures synthétiques assemblée/éclatée dans `docs/3d/captures/`.
- Revue selon `revue-par-niveau`, niveau 2 retenu pour le contrat de liaison : 2 tours, relecteur-code
  et relecteur-architecture. Deux Important vérifiés, corrigés avec tests échouant avant correctif :
  coloration des composants hors carrosserie ; textures des squelettes clonés non libérées.
  Tour 2 : aucun constat. La revue humaine et la CI distante restent à faire ; aucun push autorisé.
- Sauvegarde originale des six fichiers : 6/6 SHA-256 vérifiés dans l'archive. Les dix références
  de branches/distantes/tags préexistantes restent identiques ; branche de travail exclusivement V2.

## Point de reprise — fidélité automobile toujours prioritaire

- Les aperçus des candidats Betier (Multipla avant restylage) et Kevin Love (Valhalla de série)
  ont été inspectés ; bons candidats d'extérieur, aucune validation dimensionnelle ou mécanique.
  Le téléchargement officiel exige une authentification (401). Aucun mesh de ces voitures obtenu.
- Manques détaillés dans `docs/3d/ASSETS_NEEDED.md` : carrosseries et ouvrants réels, six places
  Multipla, groupe transversal ; Valhalla monocoque/V8/DCT, deux moteurs avant, moteur arrière,
  batterie HT, électronique, câblage et refroidissement, avec unités/repères/points de montage.
- `MANIFEST_FORMAT.md` donne la procédure d'intégration sur les IDs de la BOM réelle, les licences,
  empreintes et preuves séparées de géométrie/position. Un extérieur complet ne prouve pas ses organes.
- Étapes 4/5 : infrastructure et comportements contrôlés sur fixtures ; validation sur la BOM complète
  et les assets fidèles encore ouverte. Étapes 2/3 : fidélité automobile et éclaté mécanique non acquis.
  Étape 6 : tests locaux solides ; huit angles des modèles fidèles, collisions, FPS, Firefox/WebKit,
  CI distante et revue humaine non validés. La mission globale n'est pas déclarée terminée.
- Prochaine action dépendante : obtenir les GLB/CAD autorisés des deux bonnes versions, inspecter
  et intégrer leurs sous-arbres, produire les vrais manifestes d'assemblage et les captures de référence.
- Sauvegarde locale finale de l'incrément : `1552471`. Vérification ciblée après les corrections
  de revue : 4 scénarios desktop/mobile PASS. Inventaire et 2 tests Python PASS ; captures inspectées.
  Base temporaire arrêtée et supprimée ; livrable par fichier dans `docs/3d/VALIDATION_MANIFEST.md`.

## Reprise soutenance — 18 novembre 2026 — étape 1

- Mission utilisateur lue, branche `feat/3d-pro-v2` propre à `f0e6ab8`, règles/plan/ADR/validations relus.
- Cinq fichiers reçus et inspectés, ZIP imbriqué extrait sans exécuter les scripts des packs.
- Deux extérieurs GLB autonomes installés : Fiat Betier Models CC BY 4.0 ; Aston OUTPISTON
  CC BY-NC-SA 4.0 selon les métadonnées réelles (différent du candidat recherché le 17).
- GLTFLoader réel : axes, dimensions, essieux, hiérarchie, matériaux et textures inspectés. Conversion
  explicite vers -X avant, Y vertical ; Fiat échelle uniforme 0,89 et centrage par essieux.
- Rendu réel Chromium des deux extérieurs : aucune erreur JS, silhouettes inspectées, captures dans
  `docs/3d/captures/*-exterior-soutenance.png`. Intérieurs pas encore chargés, couverture physique zéro.
  Aucun meilleur extérieur remplacé par une forme générée. Les quatre GLB originaux restent intacts.
- Tests frontend avant modification : 46 tests PASS. Installer local à empreintes, inventaire et
  attributions dans `docs/3d/SOUTENANCE_ASSETS.md` ; aucun binaire tiers ajouté à Git.
- Base existante vide : base séparée `stp-soutenance-db`, port 55439, migrations/seed V2 existants ;
  API 18080, simulateur 18090, Vite 41769. Aucun code backend ni base utilisateur modifié.
- Prochaine tâche : intégrer une seule copie de chaque intérieur, vérifier les liaisons indicatives
  contre `seed_entities` et l'arbre réel, puis préparer les déplacements éclatés.

## Soutenance — étape 2 : intérieurs intégrés

- Base complète V2 exportée en lecture seule : 29977 nœuds Fiat, 14903 Aston, brouillon B 29981.
  Correspondances catalogue vérifiées dans `seed_entities`, puis pièces/parents/chemins dans l'API.
  Le résolveur runtime ne rapproche pas de noms : le résultat est un manifeste numérique explicite.
- Copie de travail Fiat aplatie sans changer les sommets ; Valhalla figée dans la pose effectivement
  affichée via GLTFLoader (positions, normales et tangentes skinnées). Original et licence conservés.
  Matériaux/textures extérieurs conservés. Aucun script de pack exécuté ni dépendance ajoutée.
- Une seule copie des intérieurs, groupée par occurrence ; pneus/jantes illustratifs retirés là où
  l'extérieur fourni est meilleur. Six sièges Fiat, moteur/boîte, V8/DCT, trois moteurs électriques,
  batterie, refroidissement, structure et trains intégrés avec calibration visuelle approximative.
- PBR des prototypes amélioré à partir de leur palette fournie. Intérieur masqué par défaut en vue
  normale, révélé sur demande (transparence, sélection interne, motorisation, éclaté). Cela préserve
  le bel extérieur et évite que les prototypes soient présentés comme une amélioration de l'habitacle.
- Couverture physique : Fiat 144/20108 occurrences, Aston 47/10002, en plus des groupes illustrés.
  Les fixations et les références sans liaison vérifiée restent explicitement indisponibles. Les
  associations ambiguës des ZIP sont reliées au sous-ensemble, pas inventées comme des pièces exactes.
- Copies dérivées et empreintes : `soutenance-prepared-assets.json`. Associations et preuves d'identité
  source/runtime : `soutenance-assembly-layout.json`. Un placement demeure approximatif même si l'ID
  de sélection est vérifié. Les extérieurs originaux restent inchangés et vérifiables par SHA-256.
- Sur la vraie BOM, la liste de 20000 indisponibles créait un gros DOM et des recherches quadratiques :
  détails différés, accès par Map et pages de 100. Test navigateur spécifique 20000 occurrences PASS.
- Tests frontend/build/lint PASS ; trois scénarios liés desktop PASS. Captures transparentes inspectées :
  organes dans les carrosseries, approximation visible et annoncée ; aucune erreur JavaScript.
- Prochaine tâche : déplacements mécaniques ordonnés et réversibles, isolation et cadrage de l'éclaté.

## Soutenance — étape 3 : éclatés progressifs

- Coque Fiat soulevée comme un seul ensemble (ouvrants non séparables dans le modèle fourni), roues
  natives écartées par coin, moteur/boîte/refroidissement séparés, cabine et châssis lisibles.
- Valhalla : coque, portes, capots moteur, aileron, sièges et roues natifs séparés ; V8/DCT, moteurs
  avant et batterie déplacés selon leurs groupes. Vecteurs explicites, pas de dispersion aléatoire.
- Cadrage animé de la destination de l'éclaté, grand groupe sélectionné encadré sans des centaines
  de boîtes ; focus de groupe et isolation de la sélection ajoutés. Vérification intégrée BOM à suivre.
- Les détails natifs d'habitacle Fiat restent prioritaires en vue normale et sont masqués lors de
  la lecture mécanique illustrative pour limiter les doublons. Les prototypes ne remplacent ni
  la belle carrosserie ni les roues détaillées. Copies originales toujours préservées.
- Captures réelles normales/éclatées et mesures dans `docs/3d/captures/*-{assembled,exploded}-soutenance.*`.
  Deux retours strictement égaux à la pose initiale, zéro erreur JS. Intersections majeures corrigées
  par dégagement de coque ; aucune validation de cinématique ou de collisions constructeur revendiquée.
- Frontend, build et lint PASS ; 11 parcours navigateur desktop liés et historiques PASS. Les contrôles
  Go/PR sont lancés sur une base séparée de la démonstration. Rien n'a été affaibli pour les faire passer.
- Prochaine tâche : parcours complet dans Structurer, bonnes occurrences des sièges et moteurs,
  isolation/focus, cas de remplacement et modes hybrides, puis instructions exactes de soutenance.

## Soutenance — étape 4 : parcours BOM et remplacement vérifiés

- Les brouillons vérifiés 1/2/3 ouvrent les vrais modèles dans Composants. Un registre explicite
  vérifie portée, racine et ancres ; il ne déduit aucune liaison de pièce par son nom. Le brouillon B
  possède son manifeste d'occurrences propre, distinct des IDs de la Fiat initiale (voir ADR 0041).
- Application et API Go réelles : sélection arbre → bloc moteur Fiat/V8 Aston, raycast → pneu Fiat
  et moteur électrique avant Aston, Détail, chemin déplié, isolation et focus de groupe validés.
  Les filtres thermique/électrique/hybride conservent les identités ; les trois moteurs et la batterie
  HV sont contrôlés explicitement. Aucun SDK ou contrat backend changé.
- Offre fournisseur accessible dans Détail de la vue combinée ; une offre seule conserve les UUID
  et dimensions de géométrie. Remplacement physique confirmé 50 Ah → 60 Ah sur le brouillon B :
  autre géométrie, même occurrence. Variante de boîtier **illustrative**, dimensions non constructeur.
  Référence et offre originales restaurées et relues après le test réel.
- Régressions reproduites avant correction : recherche après défilement virtualisé ; coque fantôme
  interceptant un clic intérieur ; intérieur masqué si extérieur absent ; cadrage trop éloigné après
  reset pendant l'animation. Les tests correspondants passent après correction. La caméra cadre la
  destination et le reset de la racine restitue le cadrage global assemblé.
- Suite Chromium desktop complète : **62 PASS**. Tests unitaires du registre ajoutés, aucune dépendance
  ou migration modifiée. Vérification SHA-256 des huit fichiers locaux d'origine/préparés PASS.
- Base `stp3d` conservée ; nouvelle base dédiée `stp3d_demo` semée avec les commandes existantes et
  `SIM_YEAR_DURATION=8760h` : la démonstration reste dans l'année des taux budget 2026. Aucune donnée
  utilisateur ni snapshot existant modifié. API 18080, Vite 41769, simulateur 18090.
- Prochaine tâche : terminer captures finales, navigation caméra et 0/50/100/0 sur les deux GLB réels,
  tests mobiles ciblés, moulinette PR, revue indépendante et guide `3D_DEMO_SOUTENANCE.md`.

## Soutenance — stabilisation et revue

- Parcours réel complet PASS sur les deux voitures : rotation à la souris, zoom molette, recentrage,
  éclatement 0/50/100/0 linéaire (erreur max 1,11e-16), retour strict à la pose, arbre → 3D,
  raycast → occurrence et Détail, isolation, focus de groupe et modes hybride/électrique/thermique.
  Remplacement physique et offre seule vérifiés sur brouillon B puis valeurs originales restaurées.
  Rapport final `docs/3d/captures/soutenance/validation.json` : **zéro erreur**. Captures inspectées.
- Chromium mobile : **13 PASS** sur navigation et composants liés. Suite desktop complète : 62 PASS.
- Revue niveau 1, base `f0e6ab8`, relecteur-code en contexte neuf : 1 Important vérifié, isolation d'un
  système depuis l'assemblé rendait la scène vide. Test navigateur rouge avant, vert après correction
  sur desktop et mobile (2 PASS). La révélation des intérieurs tient désormais compte du filtre système.
- Moulinette PR avant revue : verte ; contrôle PR après correction en cours. Le détecteur PostgreSQL
  du harnais marque `not_run` car `pg_isready` manque, bien que DATABASE_URL soit fournie aux tests.
  Deux scénarios ciblés relancés explicitement en mode verbeux confirment l'exécution en base :
  `TestBomSnapshotRejectsMutations` et `TestCatalogHTTPRoundTripAndPatch`, PASS sans saut.
- Dix références Git préexistantes comparées à la sauvegarde du 17 : inchangées. Aucun push/merge,
  reset/rebase ou changement des branches protégées. Originaux GLB et archive d'audit conservés.
- Prochaine tâche : second tour de revue limité au correctif, clôture des preuves et commit du guide.

## Soutenance — étape 5 livrée

- Deuxième tour limité à `0dab799..84f03ff` : **aucun constat nouveau**, défaut d'isolation confirmé
  corrigé. Bilan total : 2 tours, 1 Important corrigé avec test rouge/vert, 0 Nit, 0 pré-existant signalé.
  Niveau calculé 1 sur la base du travail du jour `f0e6ab8`. Le niveau 3 affiché en fin de rapport
  automatique compare à origin/main et inclut toute la V2 héritée ; il ne classe pas le seul diff du jour.
- `make verify LEVEL=pr` après correction : **vert** ; preuves conservées dans
  `docs/3d/soutenance-verify-pr.txt`, `soutenance-browser-evidence.txt`, `soutenance-postgres-evidence.txt`.
  Non exécutés par le harnais : gitleaks, osv-scanner, actionlint et détecteur d'intégration pg_isready
  absent. L'exécution PostgreSQL ciblée indépendante est prouvée sans saut. Quatre avertissements
  oxlint préexistants et avertissement de taille du chunk Three.js subsistent ; aucun test affaibli.
- Guide exact et parcours jury : `3D_DEMO_SOUTENANCE.md`. Galerie des captures réelles :
  `docs/3d/captures/soutenance/README.md`. Responsabilités, cas de panne/tests et limites :
  `docs/3d/VALIDATION_SOUTENANCE.md`. Inventaire/licences actualisés, huit empreintes GLB vérifiées.
- API laissée disponible sur 18080, Vite 41769, simulateur 18090 et base dédiée 55439. Relecture finale
  des trois BOM : coûts présents, zéro coût manquant ; batterie B revenue à part_id 195 / offer_id 474.
  Base utilisateur et anciens snapshots intacts. Serveur de mesure temporaire arrêté ; base de tests
  dédiée arrêtée après les contrôles. Archive d'audit : 6/6 empreintes d'origine conservées.
- Limites : intérieurs/variante/placements illustratifs, coque Fiat monobloc, couverture physique
  partielle explicitement indiquée. Ni CAD constructeur, collisions/cinématique certifiées, routages
  exacts ni modèles des milliers de fixations disponibles. Firefox/WebKit, FPS GPU de soutenance et
  CI distante **non vérifiés**. Aucun push ni merge réalisé.
- À relire humainement avant toute fusion : `viewer.ts` (visibilité/picking/cadrage), `NodePanel.tsx`
  (confirmation de référence), manifestes draft-1/2/3 et droits Valhalla CC BY-NC-SA. La démonstration
  locale est prête selon les parcours exécutés ; les travaux mécaniques exacts restent dépendants
  d'assets certifiés supplémentaires, précisément recensés dans `SOUTENANCE_ASSETS.md`.

## Corrections finales avant soutenance — reprise depuis 043df31

- Branche vérifiée propre `feat/3d-pro-v2` ; version stable précédente conservée dans l'historique.
  Aucun changement Go, PostgreSQL, dépendance ou asset prévu pour ces corrections ciblées.
- Option Réaliste retirée du sélecteur de l'application et des boutons de la page technique. Un
  ancien état React est neutralisé et la scène est réinitialisée par identité de BOM lors de la
  navigation. Les vues 3D et Arbre + 3D et les GLB fournis restent inchangés. Le chargeur historique
  reste uniquement couvert par les tests techniques existants ; aucun bouton produit ne l'active.
- Localisation explicite d'un sous-ensemble ajoutée pour les occurrences sans géométrie propre :
  ascendance BOM et géométries effectivement liées, racine exclue. Le bouton conserve l'ID de la pièce
  sélectionnée et annonce clairement que seule la localisation du sous-ensemble est affichée.
- Régressions de retrait du bouton et de localisation ajoutées ; 4 scénarios liés desktop PASS,
  tests unitaires de non-substitution conservés. Audit exhaustif et validation réelle en cours.

## Corrections finales — instances quantifiées et audit complet

- Deux liaisons Fiat réparées : #8 / pièce #60 (deux renforts de bas de caisse), #12932 / pièce #104
  (deux arbres à cames). Les meshes sources sont vérifiés comme copies translatées à 1e-6 près ;
  ils réutilisent deux placements existants sans créer de forme, sans doubler les groupes d'origine.
  Identités vérifiées aussi pour le brouillon B #44888 et #57812. Test de résolution rouge avant,
  vert après ; vecteurs éclatés appliqués aux deux instances et empreintes GLB revérifiées.
- Les trois autres GLB préparés et les quatre originaux sont inchangés ; seule l'organisation de
  l'intérieur Fiat préparé évolue. Les sources intégrales restent préservées. Corps, roues et
  matériaux externes ne sont pas modifiés. Version antérieure locale conservée dans /tmp en plus
  de l'historique et des outils de reconstitution.
- Audit API/scène : 44880 IDs uniques, 226 occurrences avec géométrie propre (157 Fiat, 69 Aston),
  tous leurs raycasts isolés passent. Pièces physiques : Fiat 146/20108, Aston 47/10002.
  Les comptes distinguent pièces physiques et groupes virtuels ; une géométrie de groupe n'est
  jamais annoncée comme celle de chacune des pièces absentes qu'il contient.
- Rapport exhaustif généré par `docs/3d/tools/audit_visibility.mjs` : chaque occurrence, identité,
  parent, pièce/libellé, manifeste, mesh réel, visibilité normale/sélection/éclaté, sélection et cause.
  Données structurées compressées dans `docs/3d/audit-final/`. Quatre captures, changements de voiture
  par le routeur et actualisation validés, sans requête CarConcept ni erreur de chargement.
- Restent volontairement non individualisés les cas sans preuve suffisante : cylindres/numéros
  Valhalla, 15 modules de kit pour 12 modules BOM, soupapes/quantités divergentes, tuyauteries sans
  parcours et radiateurs avant/latéraux portant la même clé indicative. Aucune association inventée.
- Moulinette PR verte (intégration DB explicitement not_run dans ce passage sans base de tests) ;
  revue indépendante niveau 1 depuis 043df31 : aucun Important/Nit/pré-existant. Suite navigateur
  complète en cours de confirmation après un passage perturbé par un rechargement de fichiers publics.

## Corrections finales — livraison vérifiée

- Suite complète desktop : **63 PASS** ; parcours BOM/composants mobile : **14 PASS**. Le nouveau
  scénario de localisation est séparé de celui de coque/transparence, sans retirer d'assertion ni
  augmenter les délais. Les premiers passages interrompus/surchargés ne sont pas comptés verts.
- Parcours réel final : **zéro erreur et zéro requête CarConcept**, les deux voitures en modes 3D et
  Arbre + 3D, actualisation et changement de route. Rotation/zoom, sélection bidirectionnelle, éclaté
  0/50/100/0, trois motorisations Aston, camshafts deux instances et localisations explicites vérifiés.
  Référence physique remplacée sur B puis part_id 195 / offer_id 474 restaurés ; offre seule sans
  changement géométrique. Captures dans `docs/3d/captures/corrections-finales/`.
- Comptes tous nœuds (y compris groupes) : Fiat 29977 = 157 géométrie propre + 25754 sous-ensemble
  seulement + 4066 sans géométrie/localisation fiable. Aston 14903 = 69 + 7216 + 7618.
  Pièces physiques : Fiat 20108 = 146 + 17257 + 2705 ; Aston 10002 = 47 + 4995 + 4960.
  Les pièces localisables restent sans modèle individuel, annoncé explicitement dans l'interface.
- `docs/3d/COMPOSANTS_NON_VISIBLES.md` contient les 44880 lignes d'occurrences et les causes exactes.
  Synthèse lisible, IDs des correspondances ambiguës, responsabilités et limites :
  `docs/3d/CORRECTIONS_FINALES.md`. Source et données d'audit disponibles pour reproduction.
  226 occurrences chargées ont un raycast positif après isolation/cadrage ; la visibilité globale
  peut rester occultée par d'autres pièces, d'où la commande d'isolation existante.
- 2 liaisons Fiat réparées, 52 mappings Fiat et 74 Aston encore non prouvés et volontairement refusés.
  Aucun problème de chargement, d'identité active ou de picking isolé restant détecté. Le retrait de
  Réaliste est global ; les localisations ajoutées ne sont pas comptées comme de nouveaux modèles.
- Revue niveau 1, un tour, aucun constat de correction. Moulinette PR verte ; intégration PostgreSQL,
  scanners optionnels absents, CI distante, Firefox/WebKit et GPU du jury non vérifiés dans ce passage.
  Le backend, PostgreSQL et les branches protégées sont inchangés. Aucun push/merge effectué.

## 18 novembre — rendu permanent et simulation économique (ADR 0042)

- Point de sauvegarde local `0d694cf`, depuis l'état propre `35ec7e4`, exclusivement sur
  `feat/3d-pro-v2`. Les huit assets et les sauvegardes d'audit précédentes sont conservés.
- L'utilisateur a explicitement validé l'option 1 et les hypothèses de l'ADR 0042 : simulation Go
  en mémoire, aucune écriture PostgreSQL, DCT/transmission/refroidissement communs, batterie BEV
  adaptée manquante signalée. L'ADR reste au statut formel Proposé à compléter par un humain.
- Service et route additive `POST /api/v1/boms/{id}/configuration-simulation` : une entrée de prix,
  change et réglages, trois appels au moteur existant sur les arbres actifs. Aucun montant calculé
  dans React, aucune formule historique changée. Exclusions de sous-arbres dédupliquées ; identités
  de racine/parents/pièces gardées ; prix manquant explicite et comparaison indisponible.
- Tests ciblés Go verts : calculs/quantités/communs, cotations/change, prix manquants, identités,
  chevauchements, stratégies achat/landed/full, conservation de la référence via le service et
  contrat HTTP. Golden des routes : une route ajoutée, aucune retirée.
- Vérification réelle API : référence actuelle 14904 nœuds (une pièce ajoutée depuis l'audit
  précédent, conservée). Hybride 461381.2621185521307522392595421971660 GBP ; thermique
  393970.4967355966814761266755596435380 GBP ; électrique
  369705.7356604838356815112468933765116 GBP. Réponse de référence strictement identique avant/après.
- En cours : intégration et validation navigateur des scénarios, rendu permanent, suite complète,
  captures et revue de niveau 2. Aucun résultat interrompu ou test en échec compté comme vert.
- Rendu permanent validé dans Chromium réel, Fiat et Valhalla : 3D / Arbre + 3D, navigation entre
  voitures, actualisation et réinitialisation, nouveaux extérieurs et intérieurs présents. Zéro
  requête CarConcept, erreur de page ou sélecteur Simple/Réaliste/Rendu. Captures et validation
  reproductible dans `docs/3d/captures/rendu-permanent/` et `tools/validate_permanent_components.mjs`.
- React utilise les arbres et totaux Go pour la BOM, la Synthèse et les objets actifs ; comparaison
  et liste paginée des retraits. Lecture seule pendant la simulation, retour sans écriture.
  Le contrôle indépendant « Filtre visuel 3D » est explicitement sans incidence sur le coût.
  Tests navigateur ciblés des scénarios : 2 PASS (montants API non constants, retraits, actualisation,
  sortie et prix manquant). Build TypeScript/Vite vert, profil explicite testé en logique pure.
- Parcours réel des trois motorisations désormais PASS : coûts Go, IDs/quantités/cotations/taux
  communs identiques, propulsion active visible et propulsion exclue absente, deux vues, éclatement,
  filtre visuel sans changement de coût et retour éditable. Zéro mutation API ou requête CarConcept.
  Empreinte de la réponse de référence avant/après identique :
  `b797bad590e946187c85bb92b67cb7d0a958338341198f58082316499096fc4c`.
  Captures et rapport : `docs/3d/captures/motorisations/` ; outil `tools/validate_powertrain.mjs`.
- Prix et change vérifiés par test du service : exactement un lot de cotations et un lot de taux
  pour les trois résultats (aucune lecture individuelle). Test ciblé `-race` vert.
- La moulinette initiale a hérité de la base par défaut du Makefile ; tests transactionnels annulés,
  schéma privé créé par le test interrompu identifié et supprimé. Relance sur la nouvelle base
  dédiée `stp3d_config_tests`, distincte de `stp3d_demo` et de la base habituelle. Tous les tests
  d'intégration ont passé ; un seuil existant de performance a échoué à 1,015 s pour 1 s sous
  concurrence. Relance de tous les paquets séquentiellement (`GOFLAGS=-p=1`), seuil inchangé.
