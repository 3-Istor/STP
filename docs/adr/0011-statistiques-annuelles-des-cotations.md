# 0011 — Fenêtre annuelle et précision des statistiques de prix
Statut : Accepté — Date : 2026-11-04 — Décideurs : Joe (accord explicite dans le chat)

## Contexte
`docs/features.md:233–250` demande min/max, courbe annuelle et variation depuis la dernière
cotation antérieure à la borne annuelle, ou depuis la première cotation si moins de 12 mois.
Les cas calendaires et l'arrondi de pourcentage ne sont pas spécifiés. Une division décimale
périodique ne doit pas subir la précision implicite d'une bibliothèque avant l'affichage.

## Options
1. **Calcul Go depuis les cotations, ratio exact puis arrondi de sortie** — règles reproductibles
   et testables ; référence historique séparée de la fenêtre des extrema.
2. **Pourcentage en decimal.Div avec précision implicite** — moins de code ; arrondi intermédiaire
   caché et résultat potentiellement faux au seuil de sortie.
3. **Calcul frontend en number** — simple pour les courbes ; précision et règle monétaire interdites.

## Décision
**Proposition : option 1**, portée par pricing ; l'appelant fournit l'instant simulé `as_of`.
Conserver prix courant, référence et ratio exact ; arrondir seulement le pourcentage affiché,
half-even à une décimale, pour produire notamment `+10,0 %` pour 100→110.

## Conséquences
- Borne = même date/heure UTC douze mois auparavant ; 29 février rabattu au 28 février.
  Courbe/min/max sur `[borne, as_of]` ; cotations futures exclues. Courant = dernière cotation
  ≤ as_of, ordre `(quoted_at, id)` déjà utilisé par LatestPriceQuote.
- Référence = dernière cotation strictement `< borne` avec le même départage. Si l'historique
  commence après la borne, prendre sa première cotation (instant croissant, id décroissant à
  égalité), indiquer la période réelle. Début exactement à la borne sans prix antérieur :
  référence indisponible explicitement, sans inventer un repli absent de la spec.
- `(courant − référence) × 100 / référence` conservé comme numérateur/dénominateur exacts
  (référence > 0) ; arrondi direct à la sortie par quotient/reste entier, sans float ni Div implicite.
  Restituer IDs, prix, devises, sources et dates de ces deux cotations pour reproduire le calcul.
  La référence hors fenêtre n'entre pas dans les extrema. Aucune donnée manquante remplacée par zéro.
- Couleur et signe de tendance viennent de l'écart exact : hausse rouge, baisse verte, égalité neutre.
  Une variation arrondie à zéro peut garder une tendance explicite « hausse < 0,1 % » ou « baisse
  < 0,1 % » ; ne pas afficher de signe moins sur un zéro arrondi.
- Tester fenêtre vide, première/unique cotation, borne exacte, année bissextile, référence externe,
  ex æquo temporels, pourcentage périodique et égalités d'arrondi. Des séries contrôlées ne remplacent
  pas l'intégration au flux/historique NLPF-68 et au signal SSE ADR 0003.
- Retour arrière : nouvelle règle explicite, sans modifier les cotations. Hors-ligne : mêmes données
  et instant donnent le même résultat ; multi-source exigera un choix de source approuvé, jamais
  une préférence inventée ici. Carbone/multi-usine sans impact sur cette fenêtre.
