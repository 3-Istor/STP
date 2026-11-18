# Polish UI/UX — 18 novembre 2026

Base propre `d22d17d`, branche unique `feat/3d-pro-v2`. Aucun changement Go, PostgreSQL,
contrat API, dépendance ou asset 3D. La version précédente reste dans l'historique local.

## Résultat

- Le panneau **Composants et sources** tient dans la hauteur du visualiseur : les boutons gardent
  leur taille et le contenu long défile à l'intérieur du panneau, jusqu'aux licences et à la pagination.
- **Structurer** autorise le défilement central pour toutes les voitures ; la colonne de droite
  reste défilante indépendamment. La liste des composants retirés possède son propre défilement,
  une hauteur maximale de 240 px, la pagination existante et un accès clavier.
- La vue électrique affiche **« Simulation électrique : scénario de démonstration avec hypothèses
  techniques. »** Les précisions sur la batterie BEV adaptée, les transformations non dimensionnées
  et les coûts additionnels non établis restent dans **Hypothèses et limites techniques**, fermé
  par défaut. Aucun prix ni périmètre de simulation n'a changé.
- Le fichier d'import et la révision sont dans **Avancé**, fermé par défaut. L'import, ses refus de
  portée/empreinte et le retour à la source du projet restent disponibles. Les licences et crédits
  ne sont pas retirés. Aucun mode Simple/Réaliste/Rendu n'est réintroduit dans le produit.

## Responsabilités et panne protégée

| Fichiers | Responsabilité |
|---|---|
| `web/src/features/vehicle3d/vehicle3d.css` | Hauteur du groupe de commandes bornée uniquement s'il contient les sources ; panneau réductible et défilant |
| `web/src/features/vehicle3d/Vehicle3DView.tsx` | Import, erreur d'import et révision regroupés dans Avancé |
| `web/src/shell/shell.css` | Défilement central de Structurer, y compris Fiat sans comparateur |
| `web/src/bom/bom.css`, `PowertrainPanel.tsx` | Liste de retraits bornée/paginée, région accessible au clavier, résumé électrique neutre |
| `web/src/bom/powertrain.ts` | Hypothèses techniques complètes et honnêtes, à la demande |
| `web/e2e/powertrain.spec.ts` | Deux résolutions laptop, molette réelle, bas des panneaux, pagination, hypothèses et six montages de transparence |
| `web/e2e/vehicle3d-bound.spec.ts` | Import caché par défaut puis accessible ; validations existantes conservées |
| `tools/validate_ui_polish.mjs` | Vrais modèles/prix Go, deux résolutions, sélection et éclaté, captures, aucune écriture de référence |

Panne reproduite avant correction : à 1366 × 768, les commandes atteignaient 565 px dans un
visualiseur haut de 404 px. Le conteneur du canvas pouvait défiler sous des commandes tronquées.
Le test rouge protège les limites du panneau et exige `scrollTop === 0` pour le canvas pendant
le défilement des sources ; les tests vérifient aussi les clics/paginations après mouvement réel
de molette. Les deux résolutions **1366 × 768** et **1280 × 720** passent.

Le test de transparence attend désormais le montage des contrôles après le chargement paresseux,
puis vérifie son état avec les assertions d'origine. Sa fonction, son nom et ses trois motorisations
dans la même boucle sont conservés : six montages avec les deux vues et le retour à Arbre,
aucun délai de test ou d'assertion augmenté. Le découpage exploratoire en cas indépendants a été
retiré après le signalement du contrôle de risque ; le diff final ne supprime aucune fonction de test.

## Validation et limites

- Moulinette PR, `make lint`, `make test`, Go `-race` sans base, 52 tests unitaires frontend,
  build TypeScript/Vite, govulncheck et audit npm : verts. PostgreSQL non rejoué dans cette passe
  frontend ; aucun code backend changé. Gitleaks/osv-scanner/actionlint absents, non exécutés.
- Revue niveau 1 : aucun constat sur le polish ; nouveau diff de tests/périmètre CSS relu sans
  constat. Le second rejeu indépendant a été interrompu pour éviter deux campagnes GPU simultanées
  et n'est pas compté vert. Le premier relecteur a indépendamment rejoué les quatre tests ciblés.
- Parcours réel : les six couples motorisation/résolution, les deux voitures en 3D / Arbre + 3D,
  défilements, hypothèses, contrôle d'import caché, sélection bidirectionnelle et éclaté passent.
  Zéro erreur de page, mutation API ou requête d'ancien modèle. Les empreintes des deux références
  BOM sont identiques avant/après ; les coûts Go restent 393970,50 / 369705,74 / 461381,26 GBP.
- Campagne desktop exploratoire isolée par fichier, avant restauration du regroupement d'origine :
  **71 PASS / 1 échec**. L'échec est le test technique
  `vehicle3d-viewer.spec.ts`, « multipla : chargement, éclaté réversible, sélection BOM et rendu
  au repos » : dépassement du délai de 30000 ms. Même dépassement reproduit avec le test et le CSS
  de `d22d17d`, lus depuis Git et exécutés dans `/tmp` sans modifier les fichiers du projet.
  Ce contrôle de l'ancien rendu de fixture reste inchangé ; aucun seuil n'a été relevé pour obtenir
  un résultat vert. La suite Playwright globale n'est donc pas déclarée entièrement verte.
- Parcours mobiles exploratoires concernés : **11 PASS**, y compris les trois motorisations isolées,
  les deux montages, les occurrences volumineuses et l'import avancé.
- Après restauration du regroupement d'origine : fichier `powertrain.spec.ts` **5 PASS / 1 échec**.
  Comparaison, prix manquant, panne API et les deux résolutions laptop passent ; les six montages
  du test de transparence cumulés dépassent également son délai global de 30000 ms sur ce Chromium
  logiciel. Chacun des trois modes avait passé séparément. Aucun état erroné de transparence n'est
  observé ; cette limite de temps reste signalée, pas contournée. Mobile non rejoué après ce seul
  changement de regroupement. La vérification Playwright finale n'est pas entièrement verte.
- CI distante, Firefox/WebKit et GPU du jury non vérifiés. Aucun push, PR ou merge.

Ticket local ouvert **UI-LEGACY-001** : analyser le temps du parcours de fixture Multipla
`npm --prefix web run test:e2e -- --project=chromium-desktop --workers=1 e2e/vehicle3d-viewer.spec.ts -g 'multipla : chargement'`.
Le test et ses assertions restent présents ; il atteint le focus/transparence après 30 s sur ce
Chromium logiciel. Le contrôle de référence conserve le même délai et échoue également avec le
test/CSS précédents. Cette limitation de vérification est distincte des parcours des nouveaux modèles,
validés ci-dessus ; aucun changement du moteur graphique ou du seuil n'est inclus dans ce polish.

Sources : `validation-polish-ui/` et `captures/polish-ui/validation.json`.
Lancement et parcours humain : [guide de soutenance](../../3D_DEMO_SOUTENANCE.md).

Captures réelles : [électrique, résumé fermé](captures/polish-ui/valhalla-electrique-1366x768.png),
[hypothèses ouvertes](captures/polish-ui/hypotheses-electriques-1366x768.png),
[sources et scroll en Arbre + 3D](captures/polish-ui/sources-arbre-3d-1280x720.png),
[Fiat](captures/polish-ui/fiat-arbre-3d-laptop.png).
