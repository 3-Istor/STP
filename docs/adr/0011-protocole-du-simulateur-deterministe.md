# 0011 — Protocole du simulateur déterministe
Statut : Accepté — Date : 2026-11-05 — Décideurs : Joe (accord explicite dans le chat)

## Contexte
NLPF-68, `docs/features.md:135–146,161–163` : cinq requêtes, 365 jours initiaux et des valeurs
identiques avec la même graine. L'API externe existe déjà (ADR 0005) ; le seed NLPF-67 doit
réutiliser ce générateur sans accéder au domaine par un client fournisseur.

## Options
1. Répliquer le catalogue dans priceapi — requêtes GET simples ; synchronisation supplémentaire.
2. Envoyer les définitions et le contexte au simulateur sans état — pas de copie du catalogue,
   découpage et rejeu possibles ; requêtes plus volumineuses.
3. Une séquence PRNG globale en mémoire — simple ; dépend de l'ordre des requêtes et de la reprise.

## Décision
**Proposition : option 2**, avec cinq opérations HTTP POST décrites dans
`docs/api-prix-et-devises.md`, derrière l'interface Source définie par pricing.
Le back ne dépend pas du package serveur priceapi. Le générateur pur partagé `internal/pricesim`
est une implémentation de source ; le seed peut l'utiliser puis insérer dans sa propre transaction.

## Conséquences
- `pricesim.v1` dérive SHA256 de version, graine textuelle, clé logique de série et jour UTC.
  L'entier dérivé modulo `10^12+1` donne `u = n × 10^-12` dans [0,1]. Prix = mini + (maxi−mini)×u ;
  FX = référence × (1−bande/100 + 2×bande/100×u). Aucun float ni arrondi de montant.
  La distribution n'est pas une modélisation de marché ; changer l'algorithme exige une nouvelle version.
- Graine, as_of initial à minuit UTC, version de dataset et paires/références/bandes sont explicites.
  Une bande est dans [0,100[ pour garder le taux positif. Aucun taux réel téléchargé implicitement.
- Une clé logique stable, distincte de l'ID technique si le seed le demande, rend les valeurs
  indépendantes du découpage et de l'ordre des requêtes. Les définitions normalisées sont conservées
  dans un manifeste versionné, dédupliqué par empreinte, avec leur période d'utilisation.
- Initialisation : as_of et ses 364 jours précédents. Nouvelle série : même fenêtre à sa découverte,
  sans modifier les séries existantes. Changement de bornes : application aux jours futurs seulement,
  ancien manifeste conservé. Offres actives et archives encore référencées par un brouillon cotées ;
  archives inutilisées conservées en historique, sans nouveaux prix.
- Valider définitions, devise, identité, instant et bornes aux deux frontières HTTP. Limiter les
  batches sans troncature silencieuse ; leur découpage ne change aucune valeur.
- Tests : mêmes entrées, graines distinctes, ordre/batches, ajout d'offre, bornes égales et FX,
  prix >2^53, reprise et changement de définition. Aucun comportement nouveau déclaré testé ici.
- Retour arrière : arrêter cette source en conservant cotations/manifeste ; un autre adapter peut
  fournir les mêmes ports. Hors-ligne : générateur pur réutilisable ; multi-source : clé de source
  explicite. Aucun nouveau service, package externe ou anticipation carbone/multi-usine.
