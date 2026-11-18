# BOM Studio V2 : diagnostic, vision cible et trajectoire

Statut : **Proposé** (à valider par l'équipe). Dates : 2026-11-16 → 2026-11-17. Rédigé avec Claude.
**Deux relectures indépendantes intégrées** : une vérification factuelle du code (§11) et un audit métier joué par un
agent « cost engineer automobile » (§2). Maquettes : artefact Design « BOM Studio V2 — maquettes cibles ».
Prototypes 3D : `prototype-3d/`.

---

## 1. Diagnostic

**Solide, à montrer** : backend Go modulaire (~15 000 lignes écrites à la main), montants décimaux avec devise,
snapshots immuables protégés en base, 30 ADR, `AGENTS.md` multi-IA, `.specs/`, CI DevSecOps, arbre de **29 977 nœuds**
déjà virtualisé.

**Ce qui ne va pas :**
1. **La valeur ne se voit pas** : les features P2 sont rangées dans des menus texte (`avant-bom.png`).
2. **Pas de parcours de décision** : on consulte, on n'arbitre pas.
3. **Le coût n'est pas une décision fiable.** Le total vaut dernière cotation × quantité × taux. Il n'inclut ni les
   remises de contrat, ni le coût rendu, ni l'outillage. Les tables de douane ne sont pas figées dans le snapshot.
4. **Deux langages visuels** (`avant-fournisseurs.png`), alors que la V1 « logiciel d'entreprise » avait plu au prof.
5. **Front sans structure** : pas de routeur ni de cache de données.
6. **La 3D de la branche** affiche un cube par pièce feuille, donc un nuage illisible (`avant-3d-branche.png`).
7. **Pas de budgets dans les données de démo** : aucun cockpit rouge/orange/vert n'est possible.

---

## 2. Audit métier : ce qu'un cost engineer en dirait

Un agent a joué un expert achats / cost engineering automobile et jugé chaque feature face à un simple Excel.
Ses conclusions les plus importantes :

**Le défaut de fond.** En automobile, le prix d'une pièce ne fluctue pas chaque jour. C'est un **prix contractuel**
fixé à la nomination du fournisseur, avec une baisse annuelle négociée, et il ne bouge qu'à travers des **clauses
d'indexation** (matières, change), en général chaque trimestre. Le flux de prix « en direct » est un artefact du
simulateur.
→ Le cours impose une API de prix simulée avec snapshots : on la garde, mais on la **présente comme une source de
cotations et d'indices**, pas comme un cours de bourse. On chiffre au **taux budget** de l'année, et le taux courant
(BCE) sert à mesurer l'écart.

| Feature | Valeur face à Excel | Verdict |
|---|---|---|
| Snapshots immuables et rejouables | **forte** : c'est le gel de BOM d'un jalon | garder, mettre au centre |
| Target costing (budget par sous-système) | **forte** : cœur du métier | garder, seeder des budgets |
| Pont de coût entre deux versions | **forte** : livrable des revues de programme | **priorité 1** |
| Contrôles avant publication | **forte** : revue de jalon | garder |
| Import de l'Excel avec rapport ligne par ligne | **forte** : vrai point d'entrée de l'utilisateur | garder |
| Contrats à paliers, coût rendu | forte en théorie, nulle aujourd'hui (hors totaux) | **refaire** : dans les totaux et le snapshot |
| Multi-devise | moyenne | refaire : taux budget + taux courant |
| Exposition au change | moyenne, peu coûteuse | garder |
| Anomalies (3 × médiane de catégorie) | faible : une catégorie mélange des masses | refaire en **€/kg par famille** |
| Optimiseur par alternatives | moyenne, naïf | refaire : proposition chiffrée **avec coût de changement** |
| Délais, make-or-buy | moyenne | garder, discret |
| Stock, ETP, stratégies par nœud | faible ou gadget | **couper de la démo** |
| Fourchette min/max des offres | trompeuse : toutes les pièces au maxi en même temps n'arrive jamais | couper ; garder le classement des pièces influentes |
| Indice HHI | faible | remplacer par mono-source et poids des 5 premiers |
| Deux sources de change avec quarantaine | gadget métier | couper |
| Facteurs carbone ADEME | hors sujet coût | couper (CO₂ reste affiché) |
| Vue 3D | gadget métier, effet démo | **bonus**, jamais dans le chemin critique |

**Ce qui manque**, et que l'on ajoute :
1. **Outillage et frais non récurrents amortis** sur le volume du programme (coût pièce = prix + outillage ÷ volume).
   Sans ça, comparer deux offres est faux.
2. **Suivi des pistes de réduction** : gain estimé, responsable, statut, coût de changement, lien vers le brouillon
   qui la chiffre. Fermer l'écart à la cible, c'est ça le target costing réel.
3. **€/kg par famille** pour les anomalies (should-cost paramétrique minimal).
4. **Pièces mono-source** et pays d'origine.
5. **Export** du snapshot et du pont de coût (PDF, Excel) : la direction lit des tableaux.

> **La phrase de la soutenance :** « Excel vous donne un chiffre. BOM Studio vous dit d'où il vient, ce qui a changé
> depuis la dernière version, et le refait à l'identique six mois plus tard. »

---

## 3. Parcours de démonstration (5 minutes)

1. **Importer** l'Excel de la Multipla : le rapport ligne par ligne signale les lignes sans offre, une devise inconnue,
   un cycle. Le reste est importé. (45 s)
2. **Cockpit** : la carrosserie dépasse son budget de 11 %, elle est en rouge. Écart à la cible : 826 €. (30 s)
3. **« Pourquoi ce coût ? »** : prix contractuel → remise de palier → change au taux budget → douane et transport →
   outillage amorti. (60 s)
4. **Brouillon** : on chiffre une piste (jante alu → tôle, offre vers l'UE). (45 s)
5. **Pont de coût V1 → brouillon**, décomposé en effet change, effet prix, changements d'offres, outillage. (60 s)
6. **Publier** : un contrôle bloque sur un contrat expiré. On corrige, on justifie, on publie. (45 s)
7. **Rejouer** : on modifie les tables de douane, on rouvre la V1, le chiffre est identique au centime. (15 s)

Bonus si le temps le permet : un clic sur la porte rouge en 3D ouvre la BOM au bon nœud.

---

## 4. Navigation : un parcours en 5 étapes, à la Lightroom

Lightroom sépare Bibliothèque, Développement, Export… en gardant la même photo sélectionnée. Ici, le métier a la même
forme. Ce n'est **pas un assistant linéaire** : on va et vient librement, l'ordre sert de repère.

| Étape | Question | Contenu |
|---|---|---|
| **1 Structurer** | De quoi est faite la voiture ? | Arbre, 3D, import Excel, cas d'emploi |
| **2 Chiffrer** | Combien, et pourquoi ? | Coût expliqué, budgets, sensibilité change et matières, anomalies €/kg |
| **3 Sourcer** | Chez qui, à quelles conditions ? | Pays, mono-source, poids des 5 premiers, contrats, taux budget et courant |
| **4 Arbitrer** | Que garder ? | Pistes de réduction, brouillons, pont de coût, comparaison |
| **5 Publier** | Est-ce prêt ? | Contrôles, note de décision, traçabilité, export |

Éléments communs à toutes les étapes :
- en haut, les 5 étapes avec un badge d'état (ex. « Publier 5/7 ») ;
- à gauche, le **navigateur BOM**, avec la même sélection partout ;
- en bas, la **bande des sous-systèmes** colorée par budget, comme le film de Lightroom ;
- en bas à gauche, le **bouton thème clair / sombre**, enregistré par navigateur.

Cette barre remplace le rail d'icônes à gauche de la première proposition, qui découpait par objet (BOM,
fournisseurs…) et non par étape de travail.

---

## 5. Périmètre

| Niveau | Contenu |
|---|---|
| **Socle** (doit marcher parfaitement) | Coût complet et figé (remises, coût rendu, outillage amorti, taux budget) avec cascade explicative · budgets seedés et cockpit · contrôles avant publication · pont de coût entre deux versions · import Excel avec rapport · navigation en 5 étapes, DA unique, thème clair/sombre |
| **Différenciant** | Pistes de réduction · sensibilité change et matières · mono-source et poids des 5 premiers · anomalies €/kg · export PDF/Excel |
| **Bonus** | Vue 3D (voir §6) · optimiseur avec coût de changement |
| **Coupé** | Stock, ETP et stratégies par nœud dans la démo · fourchette min/max · HHI · double source de change · ADEME · IA |

---

## 6. La vue 3D : trois niveaux de rendu

Les trois partagent la même correspondance avec la BOM (sélection, coloration, éclaté). Voir `Rendus3D` dans les maquettes.

| Niveau | Principe | Coût | Rendu |
|---|---|---|---|
| **Simple** | un objet généré par sous-ensemble (56) | faible, aucune licence | `prototype-3d-assemble.png` |
| **Technique** | châssis roulant procédural détaillé, carrosserie en radiographie | moyen | `prototype-3d-technique-eclate.png` |
| **Réaliste** | modèle sous licence + **table de correspondance** nœud 3D → sous-ensemble ; éclaté calculé automatiquement depuis les boîtes englobantes | faible en code, moyen en données | `prototype-3d-realiste*.png` |

Le modèle réaliste vient des exemples Khronos (« Car Concept », Eric Chadwick / DGG, **CC-BY 4.0**). Il a 101 nœuds
nommés, et 23 sous-ensembles y sont reliés par une table. Il est marqué **SIMILAIRE** (ce n'est pas une Multipla), les
logos sont masqués, l'attribution est affichée. Le focus « roue avant gauche » (`prototype-3d-realiste-focus-roue.png`)
éclate frein, jante et pneu dans l'axe, comme sur les vues éclatées de référence.

**Recommandation** : Simple par défaut, Réaliste en option. Technique seulement s'il reste du temps. L'audit métier
classe la 3D en gadget : elle ne remplace aucune étape du socle, elle sert à **localiser** (clic sur la pièce → BOM)
et à donner l'effet de démo.

---

## 7. Ce que change « Internet autorisé » (sans IA)

| Source | Usage | Statut |
|---|---|---|
| **API BCE** (publique, sans clé) | taux courant, comparé au taux budget figé | retenu, adapter derrière l'interface Go existante |
| **Banque mondiale, prix mensuels des matières** | 3 indices (aluminium, acier, cuivre) pour les clauses d'indexation, en fichier daté | retenu, léger |
| **Modèles 3D CC-BY** (Khronos, Sketchfab) | rendu Réaliste | retenu, fichier téléchargé et compressé, licence tracée |
| ADEME Base Empreinte | facteurs carbone versionnés | coupé (hors sujet coût) |
| Comptes en ligne, OIDC | voir §8 | à décider |

Règle inchangée : le domaine ne dépend pas du réseau. Chaque source passe par un adapter qui valide et date. Un snapshot
se rejoue sans réseau.

---

## 8. Comptes utilisateurs et revue : décision à prendre

`docs/features.md` dit aujourd'hui « outil mono-utilisateur » et « Won't : partage ou droits d'accès ». Changer cela est
une décision d'équipe (AGENTS §11), et c'est coûteux.

| Option | Ce que ça donne | Coût | Avis |
|---|---|---|---|
| **A. Mono-utilisateur + traçabilité** | à la publication : « proposé par », « validé par » (noms saisis), note et justifications conservées dans le snapshot | faible | **recommandé** (même avis que l'audit métier) |
| B. Profils sans mot de passe | sélecteur de profil, brouillons par personne, demande de revue | moyen | possible si l'équipe tient au multi-utilisateur |
| C. Vrais comptes, rôles, revue | authentification, droits, workflow d'approbation | élevé | « usine à gaz » au sens de la grille |

L'audit rappelle qu'un circuit ECR/ECO gère un changement technique, pas une décision de coût : la revue type
« pull request » est séduisante mais pas réaliste ici. L'option A apporte la traçabilité sans authentification.

---

## 9. Architecture et trajectoire

**Backend** : monolithe modulaire, hexagonal léger, **prouvé par un test d'architecture** (aucun domaine n'importe
`net/http`, `pgx` seulement dans les `*store`). Nouveaux adapters : BCE, indices matières. `viz3d` repris de la branche
d'Arthur, avec un niveau de représentation par sous-ensemble.
**Front** : `app/` (shell, 5 étapes, routes), `ui/` (tokens, thème clair/sombre), `features/<étape>`. Un routeur
(ADR) ; `three` (ADR 0031).

| Lot | Contenu | Preuve (test qui le met en défaut) |
|---|---|---|
| L0 Socle | shell 5 étapes, DA, thème, test d'architecture, budgets seedés | e2e navigation par URL ; archtest |
| L1 Coût complet | remises, coût rendu, outillage amorti, taux budget, tables figées au snapshot, cascade | tables modifiées après publication → même coût au centime |
| L2 Versions | contrôles avant publication, pont de coût, comparaison, note et traçabilité | ordre de décomposition du pont testé ; publication bloquée par un contrôle |
| L3 Import | import Excel avec rapport | fichier piégé (cycle, devise inconnue, `#REF!`) → lignes refusées, aucune importée en silence |
| L4 Différenciant | pistes de réduction, sensibilité, mono-source, €/kg, export | — |
| L5 Bonus 3D | Simple puis Réaliste | sélection 3D ↔ arbre (e2e) |

**Ligne de coupe** : L0 à L2 forment un parcours complet. L3 est fortement recommandé, c'est le fil rouge du cours
(« remplacer l'Excel de 300 Mo »). L5 ne bloque jamais la démo.

---

## 10. L'usine logicielle, avec preuves

Processus existant conservé (`.specs/`, DoR/DoD, ADR, CI), avec des **portes de vérification par agents à contexte
vierge**. Ce document en est le premier exemple :
- la **relecture factuelle** a trouvé 8 affirmations fausses dans la version 1 (§11) ;
- l'**audit métier** a fait couper 6 idées pourtant séduisantes et en a fait ajouter 5 plus réalistes (§2).

À tenir pendant le développement : un `journal.md` (obstacle → hypothèse → mesure → décision) pour le post-mortem,
le nombre de défauts trouvés par les revues avant merge, et des captures avant/après.

---

## 11. Corrections apportées par les relectures

**Version 1 → 2 (relecture factuelle)** : 29 977 nœuds et non 44 880 · arbre déjà virtualisé · chiffres d'optimiseur
inventés retirés · pas de cible CO₂ ni de masse · pas de budgets seedés · remises et coût rendu hors totaux · copie de
brouillon non transactionnelle · périmètre trop large.

**Version 2 → 3 (audit métier)** : prix contractuels et indexation au lieu d'un cours en direct · taux budget · outillage
amorti · pistes de réduction · fourchette min/max, HHI, double source de change et ADEME coupés · stock, ETP et
stratégies retirés de la démo · 3D en bonus · comptes : option A recommandée.

---

## 12. Questions pour l'équipe

1. Comptes : option A, B ou C (§8) ?
2. Validez-vous le périmètre §5 (socle / différenciant / bonus / coupé), et en particulier le retrait de stock, ETP et
   stratégies de la démo ?
3. 3D : Simple + Réaliste, et on reprend le backend `viz3d` d'Arthur ?
4. Validez-vous l'ajout d'un routeur côté front (ADR) ?
