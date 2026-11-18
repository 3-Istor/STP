# Intégration de main — 18 novembre 2026

Demande utilisateur : récupérer le dernier main et fusionner feat/3d-pro-v2, avec accord transmis du PO.
Base distante récupérée : `000dc0da38a1922324211138c934d822d36bdf29`.
Branche 3D avant intégration : `7a6d18b5bb0bff48152bf97a9b016b1a5c3d9300`.
PR : https://github.com/3-Istor/STP/pull/29.

## Changements d’intégration

- Conflit `NodePanel.tsx` : conserver les deux fonctions, remplacement physique illustratif et saisie
  des budgets hiérarchiques, avec les nouveaux détails de coûts manquants et l’édition de main.
- `internal/bom/configurations.go` : adapter l’appel au calcul de budgets de main ; fournir l’arbre
  actif et les cibles conservées, lire les taux de budgets une seule fois pour les trois scénarios.
- `internal/bom/configurations_test.go` : cas EUR/USD, conversion exacte, retrait des budgets des
  occurrences exclues, avertissement de répartition recalculé et absence d’écriture de référence.
- `internal/httpapi/configurations.go` et son test : réutiliser le corps d’erreur central avec code
  stable, en conservant le statut 422 des définitions de simulation invalides. Le test renforcé
  échoue avant correction ; le contrôle de main des erreurs sans code est conservé.
- ADR de la branche 3D renumérotées 0049 et 0050, puisque main utilise déjà 0041 et 0042 pour les
  anomalies et les budgets. Contenus de décision conservés ; références actives corrigées.
- `router.go`, contrat golden, `api.ts`, `BomTree.tsx`, `BomWorkspace.tsx` : conserver l’union des
  apports des deux branches. Les champs/API de main ne sont pas remplacés par leurs anciennes versions.

170 fichiers de main des périmètres migrations, money, snapshot, pricing, sampledata et ADR comparés
à l’octet près après résolution : identiques. Aucun changement de schéma supplémentaire, de dépendance
ou de règle de calcul monétaire ; aucune suppression/affaiblissement de test ni augmentation de délai.

## Protection des données et ressources

Sauvegarde complète des références et de leur histoire via bundle Git vérifié, et sauvegarde pg_dump
source dans `/tmp/stp-avant-merge-20261118T162216/`. Pack des huit GLB déjà vérifié dans Downloads.
Tests PostgreSQL et migrations 00016/00017 uniquement sur les bases nouvelles
`stp3d_merge_tests_20261118_162620` et `stp3d_merge_demo_20261118_162620`.
La seconde est une copie de démonstration ; l’application stable Docker sur 8001 n’a pas été redéployée.

## Validation

- Moulinette PR : toutes les étapes disponibles passent, dont Go `-race` avec intégration PostgreSQL
  effective, architecture, routes, sqlc, tidy, lint Go/TS, **62 tests unitaires frontend**, build,
  govulncheck et audit npm. Gitleaks, osv-scanner et actionlint absents : **not_run**.
  Le client `pg_isready` PostgreSQL 18 du conteneur dédié est appelé via un adaptateur local `/tmp` ;
  le harnais du dépôt reste inchangé.
- Playwright Chromium : **70 desktop + 70 mobile PASS**, avec les fonctions/délais d’origine.
  Les deux dépassements graphiques antérieurs ne se reproduisent pas dans ces campagnes.
- Navigateur avec vrais modèles et API Go fusionnée, sur la base copiée : Fiat et Aston, 3D / Arbre + 3D,
  éclaté et reset, actualisation, trois scénarios, aucun ancien modèle, aucune erreur de page.
  Aucune requête de mutation BOM ; empreintes structure/budgets/stock identiques avant/après.
- Les trois coûts constatés restent 393970,50 / 369705,74 / 461381,26 GBP sur les faits de démonstration.
- Firefox : première campagne et correction détaillées ci-dessous ; 40 tests ciblés finaux verts sur les quatre projets. CI distante : à consulter avant fusion finale. WebKit téléchargé mais bibliothèques
  système GStreamer manquantes : non exécuté à ce stade. Aucun résultat non exécuté n’est déclaré vert.

Sources : `validation-integration-main/` et `captures/integration-main/validation.json`.
Panne protégée : une exclusion conserve des budgets de composants retirés, ou perd le taux d’un budget
USD ; `TestServiceConfigurationsPreserveHierarchicalBudgets` vérifie le résultat et la référence inchangée.
À relire humainement : `NodePanel.tsx`, `configurations.go` (budgets partagés), route et ADR 0049/0050.
La fusion de la PR reste conditionnée aux contrôles distants du projet ; aucune protection n’est contournée.

## Revue et corrections complémentaires

Revue code : aucun constat, tests Go ciblés et 26 tests JS rejoués par le relecteur.
Revue architecture : 1 Important vérifié puis corrigé ; deuxième tour sur le nouveau diff : aucun constat.
Une comparaison actualisée après modification du volume ou des prix figés dans un autre onglet
mélangeait nouveaux totaux et anciennes métadonnées. La projection du scénario reprend maintenant
volume, devise, heures annuelles et snapshot de prix de son entrée capturée. Le test navigateur
échoue avant correction (1 véhicule au lieu de 100), puis passe sur les quatre projets Chromium/Firefox,
avec provenance du snapshot #73 et sans mutation de la BOM.

Première campagne Firefox complète : 138 PASS / 2 échecs du même test d’historique, présent dans main
et inchangé dans la branche 3D. Firefox remplace son about:blank initial implicite ; la précondition
est maintenant explicite, avec un chargement about:blank. Assertions d’origine conservées : longueur
exacte 2, ancre canonique puis retour vers about:blank. Essai indépendant dans /tmp : 4 PASS.
Aucun changement du routeur produit, aucune suppression d’assertion, aucun délai augmenté.

La moulinette PR après correction frontend est verte (62 unitaires et Go race sans base) ; la passe
PostgreSQL précédente couvre exactement le même code Go. Les deux résumés sont conservés séparément.
Le résumé brut pendant une fusion non commitée inclut les changements déjà issus de main lorsqu’il
compare à la base commune ; le risque propre à la PR est calculé explicitement contre origin/main : 2.

Validation finale ciblée après les deux corrections : **40 PASS**, quatre projets Chromium/Firefox,
`powertrain.spec.ts` et `router.spec.ts` ; voir `validation-integration-main/parcours-finaux-cibles.txt`.
