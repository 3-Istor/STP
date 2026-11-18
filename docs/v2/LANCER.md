# Lancer et tester la V2 (branche `v2/implementation`)

## Lancer

**Avec Docker (le plus simple)**

```bash
git fetch origin && git switch v2/implementation
make models          # modèle 3D réaliste (~12 Mo, téléchargé depuis le dépôt Khronos ; facultatif)
make up              # = docker compose up -d --build --wait  → front sur http://localhost:8000
make seed            # (Go requis) jeu de démo demo-v4 : Multipla + Valhalla, budgets, V1 publiée, brouillon B, pistes
```

`make models` avant `make up` : le modèle est copié dans l'image du front. Sans lui, la vue 3D passe
automatiquement en rendu Simple.

**Sans Docker pour le front** : `make db migrate seed`, puis `make dev` (API :8080, API de prix :8090,
Vite :5173).

Une base qui contient déjà l'ancien jeu `demo-v3` reçoit un second jeu : repartir d'une base vide
(`docker compose down -v`) pour une démo propre.

## Parcours de démo (≈ 5 min)

1. **Toutes les BOM** : la Multipla, sa V1 publiée (Arthur P. / Joe), le brouillon B.
2. **Structurer** : l'arbre en grand avec le % du budget ; « Arbre + 3D » puis « Réaliste » : portes avant en
   rouge, capot en orange. Un clic sur la porte sélectionne le nœud dans l'arbre ; double-clic sur une roue :
   focus éclaté.
3. **Détail › Pourquoi ce coût ?** sur « Porte avant nue » : prix, change au taux budget, remise, douane,
   transport, emballage, outillage amorti.
4. **Réglages de calcul** : passer de Complet à Achat et voir le total changer ; changer le profil
   (Familiale → Sportive) et voir les alternatives se reclasser.
5. **Chiffrer** : choc de change 5 / 10 %, pièces qui pèsent le plus, anomalies €/kg.
6. **Sourcer** : 44 % du coût exposé au change, poids des 5 premiers fournisseurs, contrat Arden expiré.
7. **Arbitrer** (ouvrir le brouillon B) : « Pourquoi le coût a bougé entre V1 et B », cause par cause.
8. **Publier** (brouillon B) : refus, car le contrat Arden expiré est un contrôle bloquant.

⚠️ Le temps est simulé (`SIM_YEAR_DURATION=40m` : une année en 40 minutes à partir du 01/01/2026). Le contrat
Arden expire le 31/03 simulé, soit **environ 10 minutes après le seed** : avant, la remise s'applique encore et
le brouillon B se publie. Pour la démo, lancer le seed un quart d'heure avant.

## Tests

`make lint` · `make test` (avec la base) · `cd web && npm run test:unit` · e2e Playwright :
`cd web && npx playwright test` (98 scénarios Chrome bureau et mobile passés en local le 17/11 ;
Firefox et WebKit non exécutés ici).

## Ce qui reste à décider par un humain

- **ADR 0031 à 0038 au statut Proposé** : un membre de l'équipe les relit et les passe à `Accepté`
  (AGENTS §8).
- **Décisions prises seules par les agents**, listées et justifiées dans `.specs/v2-lot-*/state.md`
  (sections Décisions), par exemple :
  - `purchase` = calcul historique sans remise ;
  - outillage non renseigné = erreur nommée ;
  - ordre de décomposition du pont de coût ;
  - une BOM sans profil garde son classement ;
  - seuil des anomalies à 2 × la médiane €/kg.
- **Limites connues** :
  - CI GitHub non exécutée ;
  - analyses non calculées sur un snapshot ;
  - en prix en direct, deux écrans peuvent lire des cotations d'instants différents ;
  - chunk 3D de 677 kB (chargé seulement à l'ouverture de la 3D) ;
  - modèle réaliste « SIMILAIRE » (ce n'est pas une Multipla), 29 sous-ensembles sans équivalent.

## Comment ça a été produit

Vision et maquettes validées par le PO, puis plan par lots (`docs/v2/plan-implementation.md`). Chaque lot a été
développé par un agent dans sa propre copie de travail (`.specs/v2-lot-a` … `v2-lot-g`), avec des tests écrits
d'abord. Après fusion, une relecture indépendante (agent à contexte vierge) a trouvé 1 problème bloquant et
4 importants. Ils sont corrigés dans le lot G, chacun avec le test qui l'aurait détecté (ADR 0038).
Captures : `docs/v2/captures/`.
