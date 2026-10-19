# Projet : BOM Studio (nom provisoire)

## Vision

Outil de conception et de pilotage de la nomenclature (BOM) d'une voiture. Le chef de projet construit la BOM pièce par pièce, ou à partir d'objectifs, et voit en temps réel le coût, les délais, l'impact CO2, la recyclabilité et les taxes. Des aides à la décision (target costing, make or buy, coût total rendu, anomalies, IA) l'aident à arbitrer entre coût, délai et écologie.

## Stack & contraintes

- Backend : Go (dernière version stable), API REST JSON
- Base de données : PostgreSQL
- Frontend : web, séparé du backend (proposition à valider : React + TypeScript + Vite, Three.js pour la 3D)
- IA : fournisseur LLM derrière une interface Go, branché plus tard (features P3)
- Montants : jamais de float. Décimal ou entier en unités mineures, toujours avec une devise ISO 4217
- Tout ce qui sert de référence (BOM publiée, prix fournisseur, taux de change) est figé dans des snapshots immuables

## Principes UI (s'appliquent à toutes les features)

- Interface claire, pas trop d'informations à l'écran, facile à utiliser
- Détails à la demande (panneau latéral, survol) plutôt que tout afficher
- Code couleur simple : vert = dans l'objectif, orange = proche, rouge = dépassé
- Les actions courantes (ajouter, déplacer, supprimer une pièce) se font en un ou deux gestes

---

## Feature : part-catalog
**Priorité** : P1
**Dépend de** : aucune

### Pourquoi
Il faut une liste des pièces qu'on peut mettre dans une BOM, avec toutes les données qui servent ensuite aux calculs de coût, délai et écologie.

### User story
En tant que chef de projet, je veux un catalogue de pièces riche, afin de construire une BOM sans ressaisir les données à chaque fois.

### Exigences
- Must : une pièce a un fournisseur, un prix d'achat dans sa devise, un coût de fabrication, un prix mini et maxi, un délai de livraison ou de production, un coût humain (heures × taux), une empreinte CO2, un taux de matière recyclée et un indicateur de recyclabilité
- Must : recherche et filtres (nom, catégorie, fournisseur, zone de la voiture)
- Should : photo ou modèle 3D attaché à la pièce
- Should : zone de la voiture (moteur, carrosserie, intérieur, châssis…) pour regrouper les pièces
- Won't : import automatique depuis un ERP (plus tard)

### Critères d'acceptation
- QUAND je crée une pièce avec un prix en USD ALORS le système DOIT conserver le montant et la devise d'origine sans conversion
- SI le prix mini est supérieur au prix maxi ALORS le système DOIT refuser la pièce avec une erreur explicite
- QUAND je filtre par zone "moteur" ALORS le système DOIT n'afficher que les pièces de cette zone

---

## Feature : bom-tree-editor
**Priorité** : P1
**Dépend de** : part-catalog

### Pourquoi
La BOM est un arbre (voiture → sous-ensembles → pièces). Il faut pouvoir la modifier facilement : ajout, suppression, déplacement, liens de parenté.

### User story
En tant que chef de projet, je veux manipuler la BOM comme un arbre, afin de structurer la voiture rapidement.

### Exigences
- Must : ajouter une pièce du catalogue sous un nœud, avec une quantité
- Must : supprimer une pièce ou un sous-ensemble (avec ses enfants, après confirmation)
- Must : déplacer une pièce vers un autre parent
- Must : calcul automatique des totaux par nœud (coût, CO2, délai le plus long) en remontant l'arbre
- Should : glisser-déposer dans l'UI, annuler / refaire
- Won't : édition simultanée par plusieurs utilisateurs

### Critères d'acceptation
- QUAND j'ajoute une pièce à 100 € en quantité 4 sous "Roues" ALORS le total de "Roues" et de tous ses parents DOIT augmenter de 400 €
- SI je déplace un nœud sous l'un de ses propres descendants ALORS le système DOIT refuser (pas de cycle)
- QUAND je supprime un sous-ensemble ALORS tous ses enfants DOIVENT être supprimés et les totaux recalculés

---

## Feature : bom-draft-and-snapshots
**Priorité** : P1
**Dépend de** : bom-tree-editor

### Pourquoi
On doit pouvoir expérimenter sans risque (brouillon), et garder des versions de référence qui ne bougent plus, y compris les prix de l'API au moment de la publication.

### User story
En tant que chef de projet, je veux un mode brouillon et des versions figées, afin de tester des idées sans perdre une version validée.

### Exigences
- Must : une BOM en brouillon est modifiable librement
- Must : publier un brouillon crée un snapshot immuable de la BOM et de ses réglages
- Must : le snapshot fige aussi les prix fournisseurs et les taux de change utilisés à ce moment
- Must : créer un nouveau brouillon à partir d'un snapshot
- Should : comparer deux snapshots (pièces ajoutées, retirées, écarts de coût)
- Won't : fusion automatique de deux brouillons

### Critères d'acceptation
- SI je tente de modifier un snapshot publié ALORS le système DOIT refuser
- QUAND le prix d'une pièce change dans le catalogue après publication ALORS le snapshot DOIT toujours afficher l'ancien prix
- QUAND je crée un brouillon depuis un snapshot ALORS il DOIT contenir exactement les mêmes pièces et quantités

---

## Feature : multi-currency-costing
**Priorité** : P1
**Dépend de** : part-catalog

### Pourquoi
Les pièces viennent de fournisseurs de différents pays. Il faut un coût total cohérent dans une devise de référence.

### Exigences
- Must : chaque BOM a une devise de référence
- Must : conversion avec des taux de change datés, enregistrés en base
- Must : aucun calcul monétaire en float
- Should : récupération des taux via une API externe, chaque récupération étant enregistrée comme snapshot
- Won't : couverture du risque de change

### Critères d'acceptation
- QUAND une BOM en EUR contient une pièce à 100 USD avec un taux de 0,92 ALORS son coût DOIT apparaître à 92,00 €
- SI aucun taux n'existe pour une devise ALORS le système DOIT signaler la pièce plutôt que l'ignorer

---

## Feature : suppliers-and-contracts
**Priorité** : P1
**Dépend de** : part-catalog

### Pourquoi
Il faut voir tous les fournisseurs, leurs pièces et leurs contrats, et tenir compte des remises.

### User story
En tant qu'acheteur, je veux un onglet fournisseurs, afin de voir d'un coup d'œil ce que chacun fournit et à quelles conditions.

### Exigences
- Must : onglet listant les fournisseurs, avec leurs pièces et leurs contrats
- Must : un contrat a des dates, des paliers de remise par volume et des conditions
- Must : note de qualité et de confiance par fournisseur
- Should : application automatique de la remise au coût de la BOM selon les quantités
- Won't : signature électronique de contrats

### Critères d'acceptation
- QUAND la quantité commandée d'une pièce atteint un palier de remise du contrat ALORS le coût DOIT appliquer la remise
- SI un contrat est expiré ALORS le système DOIT l'indiquer et ne plus appliquer sa remise

---

## Feature : target-costing
**Priorité** : P1
**Dépend de** : bom-tree-editor, multi-currency-costing

### Pourquoi
Le chef de projet a un budget cible pour la voiture. Il doit voir immédiatement quelle zone dépasse.

### User story
En tant que chef de projet, je veux fixer un budget cible réparti par sous-système, afin de savoir où je dépasse.

### Exigences
- Must : saisir un budget cible global (ex. 15 000 €)
- Must : répartir ce budget par sous-système (ex. moteur 3 000 €, intérieur 2 000 €)
- Must : chaque sous-système passe au vert, orange ou rouge selon son coût réel
- Should : proposition de répartition automatique à partir d'une BOM existante
- Won't : optimisation automatique de la BOM pour tenir le budget

### Critères d'acceptation
- QUAND une modification fait dépasser le budget d'un sous-système ALORS ce sous-système DOIT passer au rouge
- SI la somme des budgets par sous-système dépasse le budget global ALORS le système DOIT avertir

---

## Feature : stock-and-reuse
**Priorité** : P2
**Dépend de** : part-catalog

### Pourquoi
Certaines pièces sont déjà en stock ou recyclables. Les utiliser réduit le coût et le délai.

### Exigences
- Must : quantité en stock par pièce
- Must : la BOM indique quelles pièces sont couvertes par le stock
- Should : suggestion d'une pièce recyclable ou en stock équivalente
- Won't : gestion complète d'entrepôt

### Critères d'acceptation
- QUAND une pièce est en stock en quantité suffisante ALORS son délai DOIT être nul et elle DOIT être marquée "en stock"

---

## Feature : lead-time-planning
**Priorité** : P2
**Dépend de** : bom-tree-editor

### Pourquoi
Il faut savoir quand la voiture sera disponible, et pouvoir choisir une date cible.

### User story
En tant que chef de projet, je veux choisir une date de disponibilité, afin de voir les pièces qui la mettent en danger.

### Exigences
- Must : calcul de la date de disponibilité à partir des délais de livraison et de production
- Must : saisir une date cible et voir les pièces sur le chemin critique
- Should : proposer des alternatives plus rapides (autre fournisseur, stock)
- Won't : ordonnancement détaillé d'usine

### Critères d'acceptation
- QUAND la pièce la plus longue a un délai de 60 jours ALORS la disponibilité estimée DOIT être d'au moins 60 jours
- SI la date cible est impossible ALORS le système DOIT lister les pièces responsables

---

## Feature : workforce-fte
**Priorité** : P2
**Dépend de** : part-catalog, bom-tree-editor

### Pourquoi
Le coût humain doit se traduire en charge de travail (ETP) pour dimensionner les équipes.

### Exigences
- Must : calcul des ETP à partir des heures de main-d'œuvre de la BOM et d'une base horaire annuelle paramétrable
- Must : vue des ETP par sous-système
- Won't : planning nominatif des personnes

### Critères d'acceptation
- QUAND la BOM totalise 1 607 heures avec une base de 1 607 h/an ALORS le système DOIT afficher 1 ETP

---

## Feature : total-landed-cost
**Priorité** : P2
**Dépend de** : multi-currency-costing, suppliers-and-contracts

### Pourquoi
Le prix d'achat ne suffit pas. Il faut ajouter douane, transport et emballage pour comparer les origines.

### User story
En tant qu'acheteur, je veux le coût total rendu d'une pièce, afin de comparer une pièce de Chine et une pièce d'Europe de l'Est.

### Exigences
- Must : coût rendu = achat + douane (selon pays d'origine et de destination) + transport + emballage
- Must : tables de droits de douane et de coûts de transport paramétrables
- Must : comparaison côte à côte de deux origines pour une même pièce
- Won't : calcul douanier officiel certifié

### Critères d'acceptation
- QUAND je compare deux origines ALORS le système DOIT afficher le détail de chaque composante du coût rendu

---

## Feature : taxes-and-optimization
**Priorité** : P2
**Dépend de** : total-landed-cost, recyclability-index

### Pourquoi
Une fois la BOM faite, il faut voir les taxes économiques et écologiques, et les options pour les réduire.

### Exigences
- Must : synthèse des taxes économiques et écologiques de la BOM
- Must : liste d'options qui réduisent les taxes ou le délai, avec le gain estimé
- Won't : conseil fiscal

### Critères d'acceptation
- QUAND une option est proposée ALORS le système DOIT afficher son impact sur le coût, le délai et le CO2

---

## Feature : recyclability-index
**Priorité** : P2
**Dépend de** : part-catalog, bom-tree-editor

### Pourquoi
La directive VHU impose des objectifs de recyclage en fin de vie. Il faut mesurer la BOM et réduire les pénalités.

### Exigences
- Must : pourcentage de matériaux recyclés et recyclables de la voiture (pondéré par la masse)
- Must : estimation des pénalités de fin de vie
- Should : suggestion d'alternatives qui améliorent l'indice
- Won't : certification réglementaire

### Critères d'acceptation
- QUAND je remplace une pièce par une alternative plus recyclable ALORS l'indice et les pénalités DOIVENT être recalculés immédiatement

---

## Feature : make-or-buy
**Priorité** : P2
**Dépend de** : total-landed-cost, workforce-fte

### Pourquoi
Pour un sous-ensemble, il faut décider entre le fabriquer en interne ou l'acheter à un équipementier (Tier 1).

### Exigences
- Must : comparaison interne (coûts fixes, main-d'œuvre, logistique) vs achat (prix Tier 1, coût rendu)
- Must : seuil de volume à partir duquel fabriquer devient rentable
- Won't : décision automatique sans validation humaine

### Critères d'acceptation
- QUAND je lance la simulation sur un sous-ensemble ALORS le système DOIT afficher les deux coûts, l'écart et le volume d'équilibre

---

## Feature : cost-anomalies
**Priorité** : P2
**Dépend de** : part-catalog, bom-tree-editor

### Pourquoi
Repérer les pièces dont le coût est anormalement élevé par rapport à des pièces similaires de l'historique.

### Exigences
- Must : détection statistique simple (ex. écart à la médiane des pièces de la même catégorie)
- Must : surbrillance des pièces suspectes dans la BOM, avec la raison
- Should : remplacement plus tard par un modèle IA, derrière la même interface
- Won't : correction automatique du prix

### Critères d'acceptation
- QUAND une pièce coûte 3 fois la médiane de sa catégorie ALORS elle DOIT être mise en surbrillance avec l'écart affiché

---

## Feature : node-strategies
**Priorité** : P2
**Dépend de** : bom-tree-editor

### Pourquoi
Appliquer une stratégie de décision différente selon le groupe de pièces (ex. moteur : coût minimum, carrosserie : CO2 minimum).

### Exigences
- Must : attacher une stratégie (coût, délai, CO2, équilibré) à un nœud ou un groupe
- Must : les suggestions d'alternatives sous ce nœud respectent sa stratégie
- Won't : stratégies programmables par l'utilisateur

### Critères d'acceptation
- QUAND le nœud "Moteur" a la stratégie "coût minimum" ALORS les alternatives proposées sous ce nœud DOIVENT être triées par coût

---

## Feature : objective-driven-bom
**Priorité** : P3
**Dépend de** : target-costing, node-strategies

### Pourquoi
L'utilisateur peut partir d'objectifs (budget, délai, CO2, type de voiture) et obtenir une première BOM.

### Exigences
- Must : saisie des objectifs, génération d'un brouillon de BOM
- Should : s'inspirer d'autres voitures ou features existantes
- Won't : BOM générée publiée sans relecture

### Critères d'acceptation
- QUAND je génère une BOM ALORS elle DOIT être créée en mode brouillon et indiquer quels objectifs sont tenus ou non

---

## Feature : ai-assistant
**Priorité** : P3
**Dépend de** : bom-draft-and-snapshots, target-costing

### Pourquoi
Un chatbot pour modifier la BOM, sortir des KPI, trouver les pièces importantes et faire des prévisions financières.

### Exigences
- Must : le chatbot peut lire la BOM et répondre avec des KPI
- Must : les modifications proposées par le chatbot s'appliquent uniquement à un brouillon, après confirmation
- Should : prévisions financières simples
- Won't : modification d'un snapshot publié

### Critères d'acceptation
- QUAND le chatbot propose une modification ALORS l'utilisateur DOIT voir le changement avant de l'accepter

---

## Feature : ai-benchmark
**Priorité** : P3
**Dépend de** : part-catalog, total-landed-cost

### Pourquoi
Comparer une pièce au marché : prix, délais, écologie, origine.

### Exigences
- Must : benchmark d'une pièce sur prix, délai, CO2 et origine, avec les sources
- Should : exemples tirés d'autres voitures
- Won't : achat automatique

### Critères d'acceptation
- QUAND je lance un benchmark ALORS chaque valeur DOIT être accompagnée de sa source et de sa date

---

## Feature : exploded-3d-view
**Priorité** : P3
**Dépend de** : bom-tree-editor, part-catalog

### Pourquoi
Visualiser la voiture en vue éclatée et voir où se trouvent les pièces.

### Exigences
- Must : vue 3D éclatée de la voiture
- Must : surbrillance des pièces par zone (moteur, carrosserie…)
- Must : mise en évidence des pièces ajoutées à la voiture
- Should : cliquer une pièce en 3D sélectionne le nœud de la BOM
- Won't : édition de modèles 3D

### Critères d'acceptation
- QUAND je sélectionne la zone "carrosserie" ALORS seules ses pièces DOIVENT être en surbrillance
