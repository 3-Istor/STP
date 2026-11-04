# 0010 — Compatibilité entre navigateurs et tests UI reproductibles
Statut : Proposé — Date : 2026-11-04 — Décideurs : à renseigner par l'humain

## Contexte
NLPF-67 exige une preuve automatique pour chaque critère, dont les couleurs, courbes et
l'actualisation sans reload. L'utilisateur précise que l'application doit être utilisable quel
que soit le navigateur ; les tests doivent donc couvrir plusieurs moteurs, sur ordinateur et mobile.
`web/package.json` ne déclare aucun runner navigateur ; Chrome seul sur le poste ne suffit pas.

## Options
1. **Playwright Test avec Chromium, Firefox et WebKit** — tests locaux/CI sur les trois moteurs,
   profils ordinateur/mobile ; davantage de téléchargements et de temps de CI.
2. **Chromium seul** — moins coûteux, mais ne répond pas à la compatibilité demandée.
3. **Vérifications extérieures ou manuelles** — aucune dépendance ; reproduction CI insuffisante.

## Décision
**Proposition : option 1**, avec `@playwright/test` en développement et trois projets obligatoires
Chromium/Firefox/WebKit. Cible vérifiable : navigateurs modernes Chrome, Edge, Firefox, Safari et
autres navigateurs fondés sur ces moteurs, dans leurs versions stables courantes. L'application
utilise les standards web sans extension ni dépendance à Chrome ; consigner les versions réellement
testées, sans promettre toutes les versions historiques. La dépendance attend l'accord humain.

## Conséquences
- Après accord : version exacte compatible résolue et verrouillée, ajout via `npx npm@11 install
  --save-dev --save-exact @playwright/test` dans web, conformément aux règles npm du projet.
- Préparer `web/playwright.config.ts`, `web/e2e/` et script `test:e2e: playwright test` ; commande
  locale `npm --prefix web run test:e2e`, exécutant les trois projets, comme la CI. Utiliser Node 24
  pour les outils ; ce n'est pas un prérequis utilisateur de l'application. Installer les trois moteurs
  de test sans modifier les navigateurs personnels ; dépendances système soumises aux permissions.
- Ajouter profils mobiles et contrôles Chrome/Edge réels lorsqu'ils sont disponibles. WebKit et
  l'émulation mobile ne valident pas Safari/macOS ou Safari/iOS réels : prévoir ces contrôles,
  les laisser NOT RUN sans environnement compatible et ne pas conclure à une validation complète.
- Suite contrôlée avec fixtures décimales et intégration distincte sur vrai back/flux et base
  jetable, dépendant de NLPF-68/69. Des réponses mockées seules ne valident pas le ticket complet.
- Couvrir séparation des fournisseurs, +10,0 % rouge/baisse/stabilité, prix absent, panneau,
  survol/clavier, relecture REST sur SSE sans navigation et réponse tardive d'un ancien fournisseur.
  Vérifier tendance et couleur calculée.
  Couvrir formatage monétaire exact, reconnexion SSE et mise en page mobile sur les trois moteurs ;
  détails du graphique accessibles au toucher/clic/focus, sans survol obligatoire.
- Aucun autre runner ni bibliothèque graphique. Retour arrière : retirer outil/config/job dédié
  en maintenant les critères d'acceptation. Hors-ligne : préinstaller les moteurs/paquets.
  Multi-source, carbone versionné et multi-usine changeraient les fixtures, sans anticiper le pivot.

Références : [installation](https://playwright.dev/docs/intro),
[CLI](https://playwright.dev/docs/test-cli), [navigateurs](https://playwright.dev/docs/browsers),
consultées le 2026-11-04.
