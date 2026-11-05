# NLPF-68 — Décisions T1 à examiner

Branche créée : `nlpf-68` depuis `4dcdb48`. Plan approuvé par invocation de spec-execute ;
T1 reste en cours. Les six ADR ci-dessous sont **Proposé**, pas Accepté. Ce lot ne contient aucun
code, migration, nouvelle dépendance ou reprise de branche d'Arthur.

## Choix déjà confirmés par Joe

- Budgets cibles global/sous-systèmes par voiture ; production totale distincte.
- Coût unitaire calculé pour un volume de référence explicite et conservé avec le résultat.
- Reprise au dernier jour simulé validé, sans rattrapage de l'arrêt.

Ces trois choix ne sont pas soumis une seconde fois. Les détails techniques/cas limites
complémentaires ci-dessous restent à accepter avant code, conformément à AGENTS.md §6–8.

| Proposition | Choix recommandé | Conséquence à accepter |
|---|---|---|
| [0011](../../docs/adr/0011-protocole-du-simulateur-deterministe.md) | Simulateur externe sans état, contexte/manifestes explicites ; échantillon SHA256 décimal à 12 positions | Pas de copie catalogue ; historique 365 jours jusqu'à as_of inclus, nouvelles offres initialisées, évolution des bornes future seulement |
| [0012](../../docs/adr/0012-horloge-ingestion-et-repli-des-prix.md) | Tick complet transactionnel, curseur persistant ; snapshot de repli unique couvrant la demande | Une donnée invalide bloque le jour ; aucune notification avant commit ; reprise SSE par état courant et REST |
| [0013](../../docs/adr/0013-fermeture-transactionnelle-des-snapshots-de-prix.md) | Liens autorisés seulement dans la transaction de création de l'en-tête | Ajouter une métadonnée SQL ; anciens snapshots fermés sans modifier leurs faits ; plus d'ajout tardif |
| [0014](../../docs/adr/0014-conversion-exacte-par-taux-direct-ou-inverse.md) | Valeur rationnelle exacte, direct puis inverse, half-even seulement en sortie | Adapter l'agrégation du vrai moteur BOM avec Arthur ; aucune division décimale intermédiaire arrondie |
| [0015](../../docs/adr/0015-budgets-cibles-et-volume-de-reference.md) | Tables de paramètres BOM, évaluateurs purs pricing ; cas zéro et allocations imbriquées explicites | Absence ≠ zéro ; 0 voiture finançable ne devient pas volume=1 ; publication refusée sans volume effectif positif représentable |
| [0016](../../docs/adr/0016-integration-progressive-des-prix-et-de-la-bom.md) | Avancer sur pricing dans sa branche ; différer les raccordements BOM jusqu'aux accords | N'autorise aucune fusion/reprise de NLPF-69 ; versions de migration et modèles partagés restent à coordonner |

[Contrat d'API associé](../../docs/api-prix-et-devises.md) : cinq opérations externes, lectures
persistées, historiques prix/FX, snapshots, conversion, SSE et frontière budgets/BOM. Les fichiers
déjà acceptés et docs/features.md restent intacts. Aucune nouvelle migration n'a de numéro réservé.

## Responsabilités et panne de référence

- ADR 0011–0016 : options et conséquences pour chacun des six choix D1–D6 ; nouvelles propositions.
- `docs/api-prix-et-devises.md` : port, formats, erreurs et contrats futurs, explicitement non disponibles.
- `spec.md` : périmètre et tâches conservés, références concrètes des ADR ; `state.md` : suivi
  append-only de la préparation et des vérifications de documents.

Panne à prouver après accord : prix dans la mauvaise devise puis API injoignable. La transaction
n'enregistre ni lot ni tick ; le calcul utilise un snapshot fermé complet avec ses dates et motif,
ou produit une erreur sans zéro. Tests Source, PostgreSQL et navigateur prévus T3–T10, **NOT RUN**.

Les validations de ce lot portent sur documents, statuts et Git ; elles ne prouvent pas le nouveau
comportement applicatif. La clôture T1 attend l'acceptation humaine des propositions ; les raccordements
et vérifications NLPF-69 restent des conditions de livraison, même si ces ADR sont acceptées.
