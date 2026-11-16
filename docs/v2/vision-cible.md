# BOM Studio V2 : diagnostic, vision cible et trajectoire

Statut : **Proposé** (à valider par l'équipe). Date : 2026-11-16. Rédigé avec Claude.
**Relu par un agent à contexte vierge** : ses corrections factuelles sont intégrées (voir §9).
Sources : `docs/features.md`, `docs/feature_implemented.md`, `docs/critère_evaluation.md`, `docs/pivot.md`,
les ADR, le PDF du cours STP, la V1 (`3-Istor/NLPF`), la revue de Brian du 14/11, la discussion d'équipe du 16/11,
l'application lancée en local (captures `avant-*.png`), et un prototype 3D jetable (`prototype-3d-*.png`).

---

## 1. Diagnostic

### Ce qui est solide (et qu'il faut **montrer**)

| Domaine | Preuve |
|---|---|
| Backend Go modulaire | ~15 000 lignes écrites à la main (hors sqlc et tests). Domaines `bom`, `catalog`, `pricing`, `snapshot`, `supplier`, `money`. Handlers fins |
| Invariants métier | Montants décimaux avec devise, snapshots immuables (trigger SQL), reproductibilité, erreurs nommées |
| Décisions tracées | 30 ADR, `docs/pivot.md` qui chiffre le coût du pivot |
| Usine logicielle | `AGENTS.md` multi-IA, `.specs/`, DoR/DoD, CI DevSecOps (gosec, govulncheck, race, e2e Playwright) |
| Fonctions métier P1 et P2 | Catalogue, arbre, prix en direct (SSE), multi-devise, contrats, budgets cibles, stock, chemin critique, ETP, coût rendu par pièce, make-or-buy, anomalies, stratégies par nœud |
| Volumétrie | Multipla : **29 977 nœuds** (6 systèmes → 56 sous-ensembles → …), arbre **déjà virtualisé à la main** (`BomTree.tsx`) |

### Ce qui ne va pas

1. **La valeur ne se voit pas.** Les features P2 sont rangées dans des `▸ details` en texte (« Anomalies de coût : 558 »,
   « Disponibilité : 58 j »). Un jury voit un arbre et quatre chiffres (`avant-bom.png`). C'est le « pas de waow »
   dont parle l'équipe.
2. **Pas de parcours de décision.** On peut *consulter* une BOM, pas *arbitrer*. Aucune comparaison, aucun aperçu de
   l'effet d'un changement, aucune trace du pourquoi. C'est la question du prof : « quelle valeur par rapport à un Excel ? »
3. **Le coût n'est pas encore une « décision fiable » de bout en bout.** Les totaux de BOM n'appliquent ni les remises
   de contrat (elles ne sont que dans `internal/supplier`) ni le coût rendu (qui n'existe que par pièce,
   `/parts/{id}/landed-cost`). Les tables de douane et de transport sont lues en direct et **ne sont pas figées dans le
   snapshot** (`internal/bom/landed.go`). Un snapshot ne reproduit donc pas un coût rendu. C'est le défaut le plus
   important au regard du cours.
4. **Deux langages visuels.** `feature_implemented.md` parle de « même identité visuelle », mais les captures
   montrent une table dense côté BOM et de grandes cartes « SaaS » côté Fournisseurs (`avant-fournisseurs.png`).
   La V1 avait une identité « logiciel d'entreprise » que le prof avait appréciée.
5. **Front sans structure.** 3 766 lignes, sans routeur (impossible de partager un lien vers « Porte avant gauche de la
   Multipla ») ni cache de données, avec un état local dispersé.
6. **Bruit plutôt que signal.** Les anomalies ne sont pas classées par impact en euros. Elles deviennent non évaluables
   dès que les prix sont figés (`internal/bom/anomalies.go`), donc inutilisables pour comparer des snapshots.
7. **La 3D de la branche n'est pas montrable** (`avant-3d-branche.png`) : une primitive par pièce feuille donne un
   nuage de cubes illisible. Le backend (`viz3d`, génération asynchrone, `BlobStore`) est bon. C'est le
   **niveau de représentation** qui ne va pas.
8. **Données de démo incomplètes.** Aucun budget n'est seedé, alors qu'un cockpit vert/orange/rouge en a besoin.
   Le seul budget est sur le coût (ADR 0018), le délai a une date cible, et il n'existe **aucune cible CO₂ ni de masse**.

### Ce que le cours note

Le PDF et la grille disent la même chose : **« Montrez-moi la preuve »**, et le produit final est
**« une décision de coût fiable, pas un formulaire »**. Le critère démo dit **« périmètre réduit qui fonctionne
parfaitement de bout en bout, plutôt qu'une usine à gaz »**. On ne gagne donc pas en ajoutant quinze écrans, mais avec
**un parcours de décision complet et fiable**, chaque chiffre explicable, et une 3D qui sert ce parcours.

---

## 2. Vision cible

> **BOM Studio est le cockpit de décision du chef de projet véhicule. Il voit où sa voiture dépasse, comprend
> pourquoi, compare des options et publie une décision tracée.**

| Excel de 300 Mo | BOM Studio |
|---|---|
| Un chiffre sans histoire | Chaque coût se **décompose** (prix, change, remise, douane, transport) avec ses sources datées, **figées dans le snapshot** |
| Une seule version, écrasée | Versions immuables **comparées côte à côte**, décision publiée avec sa justification |
| « Et si… » = copier l'onglet | Un brouillon copié, modifié, puis comparé (bonus : optimiseur qui propose les remplacements) |
| On cherche la ligne 18 432 | **Vue 3D** : on clique la porte et on arrive dans la BOM, la voiture se colore là où le budget dépasse |
| Formules cassables | Calcul côté serveur, testé, reproductible. L'IA (bonus) ne fait que **proposer** |

### Parcours de démonstration

**Socle (doit marcher parfaitement, sans 3D ni IA) :**

1. **Cockpit.** On ouvre la Multipla. Les KPI apparaissent face aux cibles qui existent : coût par rapport au budget,
   date de disponibilité par rapport à la date cible. CO₂ et masse sont affichés comme valeurs, sans cible.
   Les sous-systèmes sont en vert, orange ou rouge, et les anomalies sont classées par impact en euros.
2. **Comprendre.** « Pourquoi ce coût ? » sur un nœud ouvre une cascade : prix de l'offre (cotation datée, snapshot),
   conversion (taux daté), remise du palier de contrat, droits de douane, transport, puis coût rendu.
   **Le même calcul est rejoué depuis un snapshot publié.**
3. **Comparer et décider.** On crée un brouillon depuis le snapshot V1, on change deux offres (par exemple jante tôle →
   jante alu, déjà défendable dans le jeu de démo), puis on compare V1 et le brouillon : KPI, écarts par sous-système,
   pièces modifiées. On publie V2 avec une note de décision.

**Ce qui fait l'effet « waow » (prévu, mais le socle doit tenir sans) :**

4. **Localiser en 3D.** La voiture est colorée par écart au budget, par sous-ensemble. Un clic sur la porte positionne
   la BOM sur « Porte avant gauche ». Un clic dans l'arbre cadre la 3D, et le sous-ensemble s'éclate.
5. **Optimiser un sous-arbre.** On applique la stratégie « coût minimum » sur *Carrosserie et ouvrants*. On obtient la
   liste des remplacements possibles par des alternatives déclarées, avec un aperçu du delta, et on l'applique à un
   brouillon en un geste.
6. **Copilote.** « Réduis le CO₂ de l'habitacle sans dépasser +200 € » appelle le même optimiseur et rend une proposition
   que l'humain applique ou non. C'est la règle du cours dans le produit : **l'IA propose, l'humain garantit**.

---

## 3. Features retenues

| # | Feature | Valeur | Base existante | Ce qu'il faut construire | Effort |
|---|---|---|---|---|---|
| F0 | **Socle UI** : une DA, un shell (rail, barre de contexte, inspecteur), un routeur avec URL profondes, les quick wins de la revue | Lisibilité, crédibilité « entreprise » | V1 (palette, densité), arbre virtualisé | Tokens, shell, routes, refonte Fournisseurs en table | M |
| F1 | **Coût fiable et expliqué** : remises et coût rendu intégrés aux totaux, tables douane/transport figées au snapshot, cascade par nœud | Cœur du cours | `supplier` (remises), `landed` (par pièce), conversion datée | Intégration aux totaux, migration snapshot, endpoint d'explication, UI cascade | **M–L** |
| F2 | **Cockpit** : KPI face aux cibles existantes, statut des sous-systèmes, anomalies classées par impact € | Voir où ça dépasse en 5 s | target-costing, planning, anomalies | Budgets seedés, classement, mise en page | S–M |
| F3 | **Page Décision** : comparer deux versions (snapshot ou brouillon), diff des pièces, note de décision à la publication | « BOM compare » demandé par l'équipe | snapshots, brouillon depuis snapshot | Diff, endpoint de comparaison, note (colonne posée à la publication, avant le trigger) | M |
| F4 | **Vue 3D cockpit** (refonte de la branche) | Localiser, effet démo | `viz3d`, ADR 0031/0032 | Voiture par sous-ensemble, focus, coloration budget | L |
| F5 | **Optimiseur de sous-arbre** (changeset) | Agir à l'échelle d'un ensemble | `suggest.go` (un seul nœud pièce), `strategy.go` (notation) | Parcours du sous-arbre, aperçu du delta, application en une transaction | M |
| F6 | **Copilote IA** derrière un port Go | IA maîtrisée | règles AGENTS §3 | Adapter LLM (clé en variable d'env), outils = F1/F5 ; désactivé sans clé | M |

**Coupé volontairement**, à dire en soutenance comme un choix et non un oubli : stress tests de change ou de douane,
carbone versionné, contrat OpenAPI, palette `Ctrl+K`, génération de BOM par IA, recherche Sketchfab automatique.

### Le modèle « scénario », et ses limites

Il n'y a pas d'objet « scénario » dans le code. Un scénario, c'est **un brouillon créé depuis un snapshot**
(`POST /snapshots/{id}/drafts`), c'est-à-dire une copie de toute la BOM. Les limites sont connues :

- on ne peut pas créer un brouillon depuis un autre brouillon ;
- la copie de 30 000 nœuds n'est **pas transactionnelle** (en cas d'échec, une suppression de rattrapage, `snapshot.go`) ;
- comparer A et B suppose donc de publier A d'abord.

On garde ce modèle pour la V2, puisqu'il marche et qu'il est simple. La copie transactionnelle est à corriger dans F3
(test qui coupe la base au milieu de la copie).

### F4 en détail : une 3D au service de la décision

Le problème est le **niveau de détail**. 30 000 nœuds ne peuvent pas devenir 30 000 objets lisibles.

- **Niveau véhicule (par défaut)** : une voiture procédurale avec **un élément par sous-ensemble** (56). Chaque élément
  porte l'identifiant du nœud de BOM. On obtient moins de 100 appels de rendu, et c'est lisible. Le prototype
  (`prototype-3d-assemble.png`) montre que c'est faisable avec `three` seul, en environ 300 lignes.
- **Niveau focus** : quand on sélectionne un sous-ensemble, le reste passe en fantôme, et **seulement alors** ses pièces
  apparaissent, éclatées et instanciées par forme (`prototype-3d-focus.png`).
- **Coloration** : par système, ou par écart au budget du sous-ensemble.
- **Honnêteté** : formes `GÉNÉRÉ`, positions estimées (ADR 0032), affichés dans l'interface.
- **Risque** : il faut écrire la correspondance entre les 56 sous-ensembles et leurs emplacements. Elle tient dans une
  table de données, sans logique. Repli : 6 systèmes seulement.

---

## 4. Direction artistique cible

**« Outil d'ingénieur, pas landing page »**, dans l'esprit des PLM et ERP denses, en reprenant la palette de la V1.

- Une seule feuille de tokens. Neutres froids, un accent bleu acier `#2f5d9e` (V1). Couleurs **sémantiques uniquement**
  (`--ok`, `--warn`, `--over`) ; le CO₂ et la 3D restent neutres sauf pour un statut.
- IBM Plex Sans avec **chiffres tabulaires**, 13 px de base, lignes de 28 à 30 px.
- Rail de navigation, barre de contexte (BOM, version, source de prix et fraîcheur), zone de travail,
  **inspecteur à droite** (détail à la demande, AGENTS §5).
- Ni grandes cartes, ni dégradés, ni ombres. Bordures de 1 px, rayons de 4 px. Fournisseurs passent en table dense
  avec une sparkline en colonne.

Les maquettes sont dans l'artefact Design associé.

---

## 5. Architecture

**Backend : on garde et on formalise.** C'est un monolithe modulaire, hexagonal léger par domaine. On le **prouve**
avec un test d'architecture (`internal/archtest`) : aucun domaine n'importe `net/http`, `pgx` n'apparaît que dans les
`*store`, et il n'y a pas de cycle entre domaines. La V1 avait ce test en Python (`test_layer_dependencies.py`).
Seul nouveau domaine : `viz3d` (repris de la branche). F3, F5 et F6 vivent dans `bom` et `snapshot`, plus un port
`Advisor` pour F6.

**Front : on structure, avec le minimum de dépendances.**
`web/src/app` (shell, routes), `web/src/ui` (tokens et composants), `web/src/features/<…>`.
Deux dépendances proposées (une ADR chacune) : un routeur (URL profondes, sélection partagée entre BOM et 3D via
`?node=`) et `three` (déjà proposé dans l'ADR 0031 ; `@react-three/fiber` reste optionnel, le prototype s'en passe).

**ADR à rédiger** (statut Proposé, acceptation humaine) : coût rendu et remises dans les totaux et le snapshot ·
DA et tokens · routeur · comparaison et note de décision · niveau de représentation 3D · copilote derrière un port.

---

## 6. Trajectoire

| Lot | Contenu | Preuve attendue (test qui le met en défaut) | Coût estimé |
|---|---|---|---|
| **L0** Socle | F0, test d'architecture, lot de données de démo (budgets seedés, paire d'alternatives défendable) | archtest échoue si un domaine importe `net/http` ; e2e de navigation par URL | 1 j |
| **L1** Coût fiable | F1 | même snapshot + tables de douane modifiées après publication = même coût rendu ; remise de palier appliquée au total | 1,5–2 j |
| **L2** Cockpit + Décision | F2 + F3 | diff de deux snapshots connus ; publication avec note refusée sur un snapshot existant (trigger) | 1,5 j |
| **L3** 3D | F4 (en parallèle de L1–L2, autre personne ou autre agent) | sélection 3D ↔ arbre (e2e) ; pas de régression de la vue BOM | 2 j |
| **L4** Optimiseur | F5 | changeset appliqué en une transaction ; aucune pièce sans alternative déclarée n'est remplacée | 1 j |
| **L5** Copilote | F6 | sans clé : fonctionnalité masquée ; avec clé : la proposition n'est jamais appliquée sans confirmation | 1 j |

**Ligne de coupe** : L0 à L2 forment un parcours complet, et c'est le minimum à présenter. L3 est le plus gros risque
technique : il passe en parallèle et ne bloque jamais la démo. L4 et L5 sont du bonus.

**Post-mortem au fil de l'eau** : un fichier `docs/v2/journal.md` tenu à chaque lot (obstacle, hypothèse, mesure,
décision). On ne le reconstitue pas à la fin.

---

## 7. Comment on produit (l'usine logicielle à montrer en soutenance)

Le processus existant (`.specs/`, DoR/DoD, ADR, CI) est bon : on ne le remplace pas, on y ajoute des **portes de
vérification** et on les **mesure**.

```
vision (ce document) ─► .specs/<lot>/spec.md (critères EARS, DoR)
  ─► plan en tâches courtes, TDD (test rouge, puis code)
  ─► implémentation par agents (un worktree par lot, lots indépendants en parallèle)
  ─► revue 1 : conformité à la spec (agent à contexte vierge)
  ─► revue 2 : qualité et sécurité (agent à contexte vierge, grille AGENTS §4)
  ─► vérification : make lint test, e2e, captures avant/après jointes à la PR
  ─► humain : accepte les ADR, relit, merge
```

**Ce document est lui-même passé par cette boucle** : un agent sans contexte l'a relu et a trouvé huit affirmations
fausses ou inventées dans la première version (§9). C'est exactement le type de preuve que demande le critère
« usine logicielle à l'ère de l'IA ».

Preuves à collecter : défauts trouvés par les revues avant merge, exemples de code IA refusé et pourquoi, temps réel
par lot comparé à l'estimation, captures avant/après.

---

## 8. Questions ouvertes pour l'équipe

1. **Date de la soutenance** et personnes disponibles : cela fixe la ligne de coupe.
2. Qui porte quel lot ? Les branches 3D d'Arthur sont-elles reprises (le backend `viz3d` vaut la peine) ?
3. Avez-vous une clé API LLM pour F6 ? Sinon F6 sort du périmètre.
4. Validez-vous l'ajout d'un routeur côté front (une ADR) ?

---

## 9. Ce que la relecture a corrigé

La première version de ce document, écrite par l'IA, contenait ces erreurs, trouvées par la relecture indépendante :

- 44 880 nœuds : c'était Multipla + Valhalla. La Multipla seule fait 29 977 nœuds, avec 56 sous-ensembles et non 59.
- Le document proposait TanStack Virtual, alors que l'arbre est déjà virtualisé.
- Le parcours citait des chiffres inventés (« 14 remplacements, −1 240 € ») pour un optimiseur qui n'existe pas :
  les suggestions ne portent que sur un seul nœud pièce.
- Le document supposait des cibles CO₂ et masse, qui n'existent pas. Aucun budget n'est seedé non plus.
- L'explication d'un coût était estimée « petite », alors que remises et coût rendu ne sont pas dans les totaux et que
  les tables de douane ne sont pas figées : c'est devenu le lot central L1.
- Il manquait la non-transactionnalité de la copie de brouillon et le fait que les anomalies ne s'évaluent pas sur
  prix figés.
- Le périmètre était trop large (9 features, 4 domaines, 4 dépendances, OpenAPI) au regard du critère « usine à gaz ».
