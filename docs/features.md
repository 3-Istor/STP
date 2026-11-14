# Projet : BOM Studio (nom provisoire)

## Vision

Outil de conception et de pilotage de la nomenclature (BOM) d'une voiture. Le chef de projet construit la BOM pièce par pièce, ou à partir d'objectifs, et voit en temps réel le coût, les délais, l'impact CO2, la recyclabilité et les taxes. Des aides à la décision (target costing, make or buy, coût total rendu, anomalies, IA) l'aident à arbitrer entre coût, délai et écologie.

## Stack & contraintes

- Backend : Go (dernière version stable), API REST JSON
- Base de données : PostgreSQL
- Frontend : web, séparé du backend : React + TypeScript + Vite, Three.js pour la 3D
- IA : fournisseur LLM derrière une interface Go, branché plus tard (features P3)
- Pas d'authentification ni de comptes utilisateurs : outil mono-utilisateur
- Volumétrie : une BOM peut atteindre 30 000 nœuds ; l'arbre reste fluide (affichage virtualisé) et le recalcul des totaux après une modification prend moins d'une seconde
- Montants : jamais de float. Décimal ou entier en unités mineures, toujours avec une devise ISO 4217
- Tout ce qui sert de référence (BOM publiée, prix fournisseur, taux de change) est figé dans des snapshots immuables

## Principes UI (s'appliquent à toutes les features)

- Interface claire, pas trop d'informations à l'écran, facile à utiliser
- Détails à la demande (panneau latéral, survol) plutôt que tout afficher
- Code couleur simple : vert = dans l'objectif, orange = proche (à partir de 90 % de l'objectif, seuil paramétrable), rouge = dépassé
- Les actions courantes (ajouter, déplacer, supprimer une pièce) se font en un ou deux gestes

## Vocabulaire (s'applique à toutes les features)

- **Pièce** : élément du catalogue, décrit indépendamment de qui la vend (masse, CO2, recyclabilité…)
- **Offre** : une pièce vendue par un fournisseur, avec sa propre devise, sa fourchette de prix, son délai, sa qualité et son pays d'origine. Plusieurs fournisseurs peuvent proposer la même pièce
- **Cotation** : un prix daté d'une offre, ou un taux de change daté, fourni par live-price-feed. **Le prix d'une offre est toujours sa dernière cotation** (ou celle du snapshot de prix choisi) : le catalogue ne stocke pas de prix d'achat
- **Alternative** : lien déclaré dans le catalogue entre deux pièces interchangeables ; sert à toutes les suggestions (stock, délai, recyclabilité, stratégies, optimisation)
- **Nœud / sous-système** : un nœud de l'arbre de la BOM. Budgets, stratégies, ETP et totaux s'attachent à des nœuds
- **Zone** : attribut de classement d'une pièce (moteur, carrosserie, intérieur, châssis…), sert à la recherche et à la vue 3D
- **Volume** : nombre de voitures produites, réglage de la BOM. Les quantités commandées = quantité par voiture × volume
- **Snapshot de prix** : fige les cotations (prix de chaque offre et taux de change) à un instant donné (live-price-feed)
- **Snapshot de BOM** : fige tout le reste au moment de la publication (structure, quantités, offres, attributs des pièces, contrats et remises, budgets, volume, réglages) et référence un snapshot de prix
- Chaque BOM est traitée de façon indépendante : rien n'est partagé ni réservé entre deux BOM (stock compris)

---

## Feature : part-catalog
**Priorité** : P1
**Dépend de** : aucune

### Pourquoi
Il faut une liste des pièces qu'on peut mettre dans une BOM, avec toutes les données qui servent ensuite aux calculs de coût, délai et écologie.

### User story
En tant que chef de projet, je veux un catalogue de pièces riche, afin de construire une BOM sans ressaisir les données à chaque fois.

### Exigences
- Must : une pièce a un nom, une catégorie, une zone de la voiture, une masse (kg), une empreinte CO2 (kg CO2e par unité), un taux de matière recyclée (%), un indicateur de recyclabilité, un coût de fabrication (montant + devise), un délai de production (jours calendaires) et un coût humain (heures × taux horaire, le taux étant un montant + devise)
- Must : une pièce a une ou plusieurs offres ; une offre a un fournisseur, une devise ISO 4217, un prix mini et maxi dans cette devise (bornes de référence de la cotation), un délai de livraison (jours calendaires), une note de qualité et un pays d'origine (ISO 3166)
- Must : le prix affiché d'une offre est sa dernière cotation live-price-feed ; aucun prix d'achat n'est saisi dans le catalogue
- Must : déclarer des alternatives entre pièces (lien symétrique « interchangeable avec »)
- Must : recherche et filtres (nom, catégorie, fournisseur, zone de la voiture)
- Must : pourcentages et notes stockés en décimal, jamais en float
- Must : une pièce ou une offre utilisée dans une BOM ne se supprime pas, elle s'archive (masquée du catalogue, toujours lisible depuis les BOM qui l'utilisent)
- Should : photo ou modèle 3D attaché à la pièce
- Should (non prioritaire) : import CSV du catalogue
- Won't : import automatique depuis un ERP (plus tard)

### Critères d'acceptation
- QUAND je crée une offre en USD ALORS ses cotations DOIVENT rester en USD, sans conversion
- SI le prix mini d'une offre est supérieur au prix maxi ALORS le système DOIT refuser l'offre avec une erreur explicite
- QUAND deux fournisseurs proposent la même pièce ALORS la pièce DOIT apparaître une seule fois avec ses deux offres
- QUAND je déclare A alternative de B ALORS B DOIT apparaître comme alternative de A
- QUAND je filtre par zone "moteur" ALORS le système DOIT n'afficher que les pièces de cette zone
- SI je supprime une pièce utilisée dans une BOM ALORS le système DOIT l'archiver et la BOM DOIT continuer à l'afficher

---

## Feature : bom-management
**Priorité** : P1
**Dépend de** : aucune

### Pourquoi
Il y a plusieurs BOM (plusieurs voitures, plusieurs variantes). Il faut pouvoir les créer, les retrouver et régler leurs paramètres globaux.

### User story
En tant que chef de projet, je veux gérer une liste de BOM, afin de travailler sur plusieurs voitures en parallèle.

### Exigences
- Must : créer, lister, renommer une BOM ; supprimer une BOM qui n'a aucun snapshot publié
- Must : réglages d'une BOM : devise de référence (ISO 4217) et volume (nombre de voitures), modifiables à tout moment sur un brouillon
- Should (non prioritaire) : export CSV / PDF d'une BOM ou d'un snapshot
- Won't : partage ou droits d'accès (pas d'authentification)

### Critères d'acceptation
- QUAND je crée une BOM ALORS elle DOIT avoir une racine, une devise de référence et un volume
- SI je tente de supprimer une BOM qui a un snapshot publié ALORS le système DOIT refuser

---

## Feature : bom-tree-editor
**Priorité** : P1
**Dépend de** : bom-management, part-catalog, live-price-feed, multi-currency-costing

### Pourquoi
La BOM est un arbre (voiture → sous-ensembles → pièces). Il faut pouvoir la modifier facilement : ajout, suppression, déplacement, liens de parenté.

### User story
En tant que chef de projet, je veux manipuler la BOM comme un arbre, afin de structurer la voiture rapidement.

### Exigences
- Must : ajouter une pièce du catalogue sous un nœud, avec une quantité par voiture et l'offre choisie par l'utilisateur (pas de choix automatique)
- Must : changer l'offre d'une pièce dans la BOM (autre fournisseur) en un geste
- Must : supprimer une pièce ou un sous-ensemble (avec ses enfants, après confirmation)
- Must : déplacer une pièce vers un autre parent
- Must : calcul automatique des totaux par nœud en remontant l'arbre : coût (chaque montant est d'abord converti dans la devise de référence de la BOM, puis additionné), CO2, et délai = délai le plus long des enfants + délai de production du nœud
- Must : les quantités se multiplient le long du chemin (quantité réelle = produit des quantités de la racine à la pièce)
- Should : glisser-déposer dans l'UI, annuler / refaire
- Won't : édition simultanée par plusieurs utilisateurs

### Critères d'acceptation
- QUAND j'ajoute une pièce à 100 € en quantité 4 sous "Roues" ALORS le total de "Roues" et de tous ses parents DOIT augmenter de 400 €
- QUAND "Roue" a la quantité 4 et contient "Écrou" en quantité 5 à 1 € ALORS la voiture DOIT compter 20 écrous et 20 € d'écrous
- QUAND un sous-ensemble a 2 jours de production et que son enfant le plus long a 30 jours de délai ALORS son délai DOIT être de 32 jours
- QUAND une BOM en EUR contient une pièce à 100 € et une pièce à 100 USD (taux 0,92) ALORS le total DOIT être de 192,00 €
- SI je déplace un nœud sous l'un de ses propres descendants ALORS le système DOIT refuser (pas de cycle)
- QUAND je supprime un sous-ensemble ALORS tous ses enfants DOIVENT être supprimés et les totaux recalculés

---

## Feature : live-price-feed
**Priorité** : P1
**Dépend de** : part-catalog

### Pourquoi
Les prix des pièces et les taux de change entre devises fluctuent en continu. Ils viennent d'une API de prix externe (simulée pour l'instant), qui sert aussi aux conversions de devises. Pour comparer, décider et publier, il faut pouvoir figer ces prix et ces taux à un instant donné.

### User story
En tant que chef de projet, je veux voir les prix et les taux de change actuels et pouvoir les figer dans un snapshot de prix, afin de travailler sur des chiffres stables et traçables.

### Exigences
- Must : l'API de prix est appelée derrière une interface Go, avec trois requêtes de prix : le prix d'une offre précise, les prix de toutes les offres d'un fournisseur, les prix de toutes les offres
- Must : la même API fournit les taux de change, avec deux requêtes : le taux d'une paire de devises (ex. USD → EUR), tous les taux disponibles
- Must : chaque prix renvoyé est un montant décimal avec sa devise ISO 4217, la source et l'horodatage de la cotation
- Must : chaque taux renvoyé est un décimal avec sa paire de devises ISO 4217, la source et l'horodatage de la cotation
- Must : chaque prix et chaque taux récupérés sont enregistrés en base comme des cotations datées ; une cotation n'est jamais modifiée ni écrasée (historique)
- Must : créer un snapshot de prix (une offre, un fournisseur, les offres d'une BOM ou tout le catalogue) qui fige les cotations de prix du moment **et** les taux de change nécessaires pour les convertir ; un snapshot de prix est immuable
- Must : un calcul de coût indique toujours sur quels prix et quels taux il repose : en direct (dernières cotations) ou snapshot de prix choisi
- Must : un simulateur implémente l'interface et il est déterministe avec une graine (tests reproductibles) :
  - les prix fluctuent dans la fourchette prix mini / prix maxi de l'offre
  - les taux fluctuent autour d'un taux de référence par paire, dans une bande paramétrable (ex. ±5 %)
  - au premier démarrage, il génère un historique d'un an (une cotation par jour et par offre, et par paire de devises), enregistré comme des cotations normales
  - ensuite, le temps est accéléré : une année simulée se déroule en 40 minutes réelles (vitesse paramétrable), soit une journée simulée toutes les ~6,6 s ; les cotations sont horodatées en temps simulé
- Must : rafraîchissement périodique des prix et des taux en direct, au rythme de l'horloge simulée ; les clients (UI) sont notifiés des nouvelles cotations
- Must : lire l'historique des cotations d'une offre ou d'une paire de devises sur une période (sert aux courbes de la page fournisseur)
- Won't : brancher une vraie API fournisseur ou de change (plus tard, nouvelle implémentation de la même interface)
- Won't : prévision de l'évolution des prix ou des taux

### Critères d'acceptation
- QUAND je demande le prix d'une offre ALORS le système DOIT renvoyer le montant, la devise et l'horodatage de la cotation
- QUAND je demande les prix d'un fournisseur ALORS le système DOIT renvoyer une cotation pour chacune de ses offres et aucune pour les autres
- QUAND je demande le taux USD → EUR ALORS le système DOIT renvoyer le taux, la paire et l'horodatage de la cotation
- QUAND je crée un snapshot de prix puis que l'API renvoie un nouveau prix ou un nouveau taux ALORS le snapshot DOIT toujours contenir l'ancien prix et l'ancien taux
- SI je tente de modifier un snapshot de prix ou une cotation ALORS le système DOIT refuser
- SI un brouillon utilise un snapshot de prix qui ne couvre pas toutes ses offres ALORS le système DOIT refuser et lister les offres manquantes
- QUAND le simulateur tourne à la vitesse par défaut ALORS 40 minutes réelles DOIVENT produire 365 jours simulés de cotations
- SI l'API ne renvoie pas de prix pour une offre, ou pas de taux pour une devise ALORS le système DOIT signaler la pièce (pas de prix silencieusement à zéro, pas de conversion ignorée)
- QUAND le simulateur tourne avec la même graine ALORS il DOIT produire la même suite de prix et de taux, les prix toujours entre le prix mini et le prix maxi, les taux toujours dans leur bande
- QUAND le système démarre pour la première fois avec le simulateur ALORS chaque offre DOIT avoir un an de cotations quotidiennes
- QUAND le système redémarre ALORS l'historique déjà généré NE DOIT PAS être regénéré ni dupliqué

---

## Feature : bom-draft-and-snapshots
**Priorité** : P1
**Dépend de** : bom-tree-editor, live-price-feed, multi-currency-costing

### Pourquoi
On doit pouvoir expérimenter sans risque (brouillon), et garder des versions de référence qui ne bougent plus, y compris les prix de l'API au moment de la publication.

### User story
En tant que chef de projet, je veux un mode brouillon et des versions figées, afin de tester des idées sans perdre une version validée.

### Exigences
- Must : une BOM en brouillon est modifiable librement
- Must : un brouillon travaille soit en prix en direct (les totaux bougent à chaque cotation), soit sur un snapshot de prix choisi ; un geste « figer les prix » crée un snapshot de prix des offres de la BOM et l'attache au brouillon, un autre le remet en direct
- Must : publier un brouillon crée un snapshot de BOM immuable qui fige tout : structure, quantités, offres choisies, attributs des pièces (CO2, masse, délais, coûts), contrats et remises appliqués, budgets, volume et réglages
- Must : le snapshot de BOM fige aussi les prix et les taux : il référence un snapshot de prix (live-price-feed), créé à la publication si le brouillon travaillait en prix en direct
- Must : créer un nouveau brouillon à partir d'un snapshot
- Should : comparer deux snapshots (pièces ajoutées, retirées, écarts de coût)
- Won't : fusion automatique de deux brouillons

### Critères d'acceptation
- SI je tente de modifier un snapshot publié ALORS le système DOIT refuser
- QUAND une nouvelle cotation arrive pour une offre après publication ALORS le snapshot DOIT toujours afficher l'ancien prix
- QUAND je modifie le CO2 d'une pièce ou la remise d'un contrat après publication ALORS le snapshot DOIT toujours afficher les anciennes valeurs
- QUAND je fige les prix d'un brouillon ALORS ses totaux NE DOIVENT PLUS changer à l'arrivée de nouvelles cotations
- QUAND je crée un brouillon depuis un snapshot ALORS il DOIT contenir exactement les mêmes pièces, offres, quantités et le même volume

---

## Feature : multi-currency-costing
**Priorité** : P1
**Dépend de** : part-catalog, live-price-feed

### Pourquoi
Les pièces viennent de fournisseurs de différents pays. Il faut un coût total cohérent dans une devise de référence.

### Exigences
- Must : chaque BOM a une devise de référence
- Must : conversion avec les taux de change datés fournis par l'API de prix (live-price-feed) : dernière cotation en direct, ou taux du snapshot de prix choisi
- Must : chaque montant converti indique le taux utilisé et sa date
- Must : un taux peut être inversé (EUR → USD = 1 ÷ USD → EUR) ; pas de conversion via une devise pivot
- Must : aucun calcul monétaire en float
- Won't : couverture du risque de change

### Critères d'acceptation
- QUAND une BOM en EUR contient une pièce à 100 USD avec un taux de 0,92 ALORS son coût DOIT apparaître à 92,00 €
- SI aucun taux n'existe pour une devise ALORS le système DOIT signaler la pièce plutôt que l'ignorer
- QUAND seul le taux USD → EUR existe ALORS une conversion EUR → USD DOIT utiliser son inverse
- SI seuls USD → EUR et EUR → JPY existent ALORS une conversion USD → JPY DOIT être signalée comme impossible

---

## Feature : suppliers-and-contracts
**Priorité** : P1
**Dépend de** : part-catalog, live-price-feed

### Pourquoi
Il faut voir tous les fournisseurs, leurs pièces et leurs contrats, et tenir compte des remises. Comme les prix fluctuent, l'acheteur doit aussi voir où en est le prix de chaque pièce et comment il a évolué sur l'année.

### User story
En tant qu'acheteur, je veux un onglet fournisseurs, afin de voir d'un coup d'œil ce que chacun fournit, à quelles conditions et à quel prix en ce moment.

### Exigences
- Must : onglet listant les fournisseurs, avec leurs pièces et leurs contrats
- Must : un contrat a des dates, des paliers de remise par volume et des conditions ; la quantité comparée aux paliers est la quantité par voiture × le volume de la BOM
- Must : si plusieurs contrats actifs couvrent une offre, l'utilisateur choisit celui qui s'applique ; la remise du palier atteint s'applique à toute la quantité (pas par tranche)
- Must : note de qualité et de confiance par fournisseur
- Must : page fournisseur listant ses offres ; pour chaque offre, le prix en direct (dernière cotation, dans sa devise d'origine), le plus haut et le plus bas des 12 derniers mois, la variation sur 12 mois en pourcentage (par rapport à la dernière cotation antérieure à la date d'il y a 12 mois, en temps simulé) et une mini-courbe (sparkline) de l'année
- Must : la page se met à jour quand de nouvelles cotations arrivent, sans recharger
- Must : au clic sur une pièce, un panneau latéral affiche le graphique détaillé du prix sur 12 mois, avec le prix actuel, le minimum, le maximum et la date de chaque cotation au survol
- Must : variation colorée : vert si le prix a baissé, rouge s'il a augmenté, neutre si stable
- Must : la variation et les extrêmes sont calculés en décimal à partir des cotations enregistrées (aucun float)
- Should : application automatique de la remise au coût de la BOM selon les quantités
- Should : afficher les prix convertis dans une devise choisie (taux en direct de live-price-feed)
- Won't : signature électronique de contrats
- Won't : alertes automatiques sur les variations de prix

### Critères d'acceptation
- QUAND la quantité commandée d'une pièce atteint un palier de remise du contrat ALORS le coût DOIT appliquer la remise
- SI un contrat est expiré ALORS le système DOIT l'indiquer et ne plus appliquer sa remise
- QUAND 1 000 pièces à 10 € atteignent un palier de 5 % à partir de 500 ALORS le coût DOIT être de 9 500 € (remise sur les 1 000)
- QUAND j'ouvre la page d'un fournisseur ALORS je DOIS voir toutes ses pièces, et seulement les siennes, chacune avec son prix en direct et sa variation sur 12 mois
- QUAND une pièce valait 100 € il y a 12 mois et vaut 110 € aujourd'hui ALORS la variation DOIT afficher +10,0 % en rouge
- QUAND une nouvelle cotation arrive pour une pièce affichée ALORS son prix et sa variation DOIVENT se mettre à jour sur la page
- SI une pièce a moins de 12 mois d'historique ALORS la variation DOIT être calculée depuis la première cotation et indiquer la période réelle

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
- Must : au lieu de saisir le volume, saisir un budget de production total ; le système en déduit le nombre de voitures = partie entière de (budget ÷ coût d'une voiture), recalculé quand le coût change
- Should : proposition de répartition automatique à partir d'une BOM existante
- Won't : optimisation automatique de la BOM pour tenir le budget

### Critères d'acceptation
- QUAND une modification fait dépasser le budget d'un sous-système ALORS ce sous-système DOIT passer au rouge
- SI la somme des budgets par sous-système dépasse le budget global ALORS le système DOIT avertir
- QUAND le budget de production est de 1 000 000 € et qu'une voiture coûte 15 000 € ALORS le volume DOIT être de 66 voitures

---

## Feature : sample-dataset
**Priorité** : P1
**Dépend de** : bom-management, part-catalog, bom-tree-editor, suppliers-and-contracts

### Pourquoi
Pour développer, tester la tenue en charge et faire une démo, il faut un jeu de données réaliste et volumineux, sans le saisir à la main.

### User story
En tant que développeur ou démonstrateur, je veux charger des voitures d'exemple complètes, afin de travailler sur des données proches du réel.

### Exigences
- Must : générateur déterministe (graine) d'un catalogue et de BOM d'exemple, le plus exhaustif possible : au moins la Fiat Multipla, d'autres modèles à ajouter
- Must : chaque voiture d'exemple a une arborescence réaliste (sous-systèmes → sous-ensembles → pièces) jusqu'à ~30 000 nœuds
- Must : toutes les données des pièces sont renseignées (masse, CO2, recyclabilité, délais, coûts, zone, catégorie), avec plusieurs devises, plusieurs fournisseurs et plusieurs offres par pièce, des alternatives, des contrats avec paliers
- Must : chargement par une commande (ex. `make seed`), idempotent
- Won't : données réelles de constructeur

### Critères d'acceptation
- QUAND je lance le chargement deux fois avec la même graine ALORS la base DOIT contenir exactement les mêmes données, sans doublon
- QUAND le chargement est terminé ALORS la BOM Fiat Multipla DOIT s'ouvrir dans l'éditeur avec ses totaux calculés

---

## Feature : stock-and-reuse
**Priorité** : P2
**Dépend de** : part-catalog

### Pourquoi
Certaines pièces sont déjà en stock ou recyclables. Les utiliser réduit le coût et le délai.

### Exigences
- Must : quantité en stock par pièce, propre à chaque BOM (aucun partage ni réservation entre BOM)
- Must : la BOM indique quelles pièces sont couvertes par le stock, totalement ou partiellement (quantité couverte affichée)
- Should : suggestion d'une pièce recyclable ou en stock équivalente
- Won't : gestion complète d'entrepôt

### Critères d'acceptation
- QUAND une pièce est en stock en quantité suffisante ALORS son délai DOIT être nul et elle DOIT être marquée "en stock"
- SI le stock ne couvre qu'une partie de la quantité ALORS le délai DOIT rester celui de l'offre et la quantité couverte DOIT être affichée

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
- Should : proposer des alternatives plus rapides (autre offre, pièce alternative, stock)
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
- Must : calcul des ETP = heures de main-d'œuvre par voiture × volume ÷ base horaire annuelle paramétrable
- Must : vue des ETP par sous-système
- Won't : planning nominatif des personnes

### Critères d'acceptation
- QUAND la BOM totalise 16,07 heures par voiture, avec un volume de 100 voitures et une base de 1 607 h/an ALORS le système DOIT afficher 1 ETP

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
- Must : comparaison côte à côte de deux offres (origines) pour une même pièce
- Won't : calcul douanier officiel certifié

### Critères d'acceptation
- QUAND je compare deux origines ALORS le système DOIT afficher le détail de chaque composante du coût rendu

---

## Feature : taxes-and-optimization
**Priorité** : P4 (plus tard : la liste des taxes et leurs formules sont à définir avant de la spécifier)
**Dépend de** : total-landed-cost

### Pourquoi
Une fois la BOM faite, il faut voir les taxes économiques et écologiques, et les options pour les réduire.

### Exigences
- Must : synthèse des taxes économiques et écologiques de la BOM
- Must : liste d'options qui réduisent les taxes ou le délai, avec le gain estimé
- Won't : conseil fiscal

### Critères d'acceptation
- QUAND une option est proposée ALORS le système DOIT afficher son impact sur le coût, le délai et le CO2

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
- Must : détection statistique simple : écart à la médiane des pièces de la même catégorie, seuil paramétrable (3× par défaut), uniquement si la catégorie compte au moins 5 pièces
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
- Must : attacher une stratégie (coût, délai, CO2, équilibré) à un nœud ou un groupe ; « équilibré » = moyenne à poids égaux des scores coût, délai et CO2, chacun normalisé entre 0 et 1 sur les alternatives comparées
- Must : les suggestions d'alternatives sous ce nœud respectent sa stratégie
- Won't : stratégies programmables par l'utilisateur

### Critères d'acceptation
- QUAND le nœud "Moteur" a la stratégie "coût minimum" ALORS les alternatives proposées sous ce nœud DOIVENT être triées par coût

---

## Feature : objective-driven-bom
**Priorité** : P4 (plus tard : « type de voiture » et l'interface IA à définir)
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
**Priorité** : P4 (à décider en fin de projet : la source des données reste à choisir)
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
**Priorité** : P4 (plus tard : rendu sans modèle 3D et sens de « pièces ajoutées » à définir)
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
