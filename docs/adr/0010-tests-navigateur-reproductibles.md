# 0010 — Tests navigateur reproductibles pour la page fournisseur
Statut : Proposé — Date : 2026-11-04 — Décideurs : à renseigner par l'humain

## Contexte
NLPF-67 exige une preuve automatique pour chaque critère, dont les couleurs, courbes et
l'actualisation sans reload. `web/package.json` ne déclare aucun runner navigateur. Chrome est
présent sur le poste, ce qui ne fournit pas à lui seul des tests versionnés exécutables en CI.

## Options
1. **Playwright Test en dépendance de développement, Chromium seul** — mêmes tests locaux/CI,
   assertions navigateur et traces ; ajoute un paquet npm et un navigateur de test à télécharger.
2. **Outil navigateur extérieur au dépôt** — utile pour vérifier ponctuellement l'écran sans
   dépendance ; disponibilité et reproduction CI non garanties.
3. **Services/API seuls + revue manuelle** — aucun nouvel outil ; insuffisant pour les critères UI.

## Décision
**Proposition : option 1**, limitée à `@playwright/test` en développement et un projet Chromium,
car elle rend les critères UI rejouables par l'équipe. Aucun ajout n'est autorisé par le seul
statut Proposé : l'accord humain sur la dépendance précède toute modification de package/lockfile.

## Conséquences
- Après accord : version exacte compatible résolue et verrouillée, ajout via `npx npm@11 install
  --save-dev --save-exact @playwright/test` dans web, conformément aux règles npm du projet.
  Pas d'initialisation automatique qui réécrive des configurations ; configuration ciblée.
- Préparer `web/playwright.config.ts`, `web/e2e/` et script `test:e2e: playwright test` ; commande
  locale `npm --prefix web run test:e2e`. Utiliser Node 24 comme la CI. Installer Chromium selon
  la documentation de la version verrouillée ; l'installation système de dépendances demande les
  permissions correspondantes, sans sudo implicite. Ne modifier aucun job existant pour le masquer.
- Première suite : serveur web local avec réponses API/SSE contrôlées et fixtures décimales.
  Suite d'intégration distincte avec vrai back/flux et base jetable ; sa disponibilité dépend de
  NLPF-68/69. Une suite mockée seule ne valide pas le ticket complet.
- Couvrir séparation des fournisseurs, +10,0 % rouge/baisse/stabilité, prix absent, panneau,
  survol/clavier, relecture REST sur SSE sans navigation et réponse tardive d'un ancien fournisseur.
  Vérifier tendance et couleur calculée dans le navigateur, pas seulement un nom de classe CSS.
  Capturer les échecs sans retries destinés à transformer un test instable en preuve de succès.
- Aucune bibliothèque de graphiques ni second runner dans cette décision. Retour arrière : enlever
  runner/config/job dédié sans affaiblir silencieusement l'acceptation. Hors-ligne : navigateur et
  paquets doivent être déjà présents ; pas de code produit anticipant le pivot. Multi-source,
  carbone versionné et multi-usine changeraient les fixtures, pas la frontière de l'outil.

Références : [installation](https://playwright.dev/docs/intro),
[CLI](https://playwright.dev/docs/test-cli), consultées le 2026-11-04.
