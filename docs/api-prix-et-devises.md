# NLPF-68 — Contrats proposés pour prix, devises et budgets

**Statut : contrat retenu, ADR 0011–0016 acceptées par Joe le 2026-11-05.** Ce document ne décrit
pas encore des routes toutes disponibles. Acceptation explicite « accepte toutes les adere et continue stp » ; ADR 0011–0016 acceptées ; ADR 0002–0005 et 0008/0010 existantes inchangées.
Les trois choix métier du 2026-11-05 sont acquis : budgets cibles par voiture, coût au volume de
référence conservé, reprise au dernier jour validé sans rattrapage.

## Frontières et ports

| Composant | Responsabilité proposée |
|---|---|
| `pricing.Source` | Cinq opérations sur des définitions et un contexte explicites, sans connaître HTTP ni pgx |
| `internal/pricesim` | Implémentation pure partagée : hash/échantillon, prix/taux et historique ; aucune horloge, DB ou requête réseau |
| `internal/priceapi` | Handlers fins du processus externe, appellent pricesim ; ne dupliquent pas le catalogue |
| `pricing/priceclient` | Adapter HTTP implémentant Source, timeout/validation, aucune dépendance du domaine vers lui |
| `pricing` et `pricingstore` | Initialisation, progression, état source, lectures, snapshots et provenance ; SQL/transactions seulement dans le store |
| `money` puis `pricing` | Valeur exacte/arrondi ; résolution direct/inverse et explication ; aucun chemin via pivot |
| `bom` et `snapshot` | Possèdent paramètres de brouillon/publication ; consomment les ports pricing et les évaluateurs purs de budgets |
| `httpapi`, UI | Traduction des erreurs/JSON et affichage ; SSE comme signal, relecture REST ; aucun calcul de montant côté TS |

Le câblage cmd/server appelle la source externe par HTTP. Le cmd/seed futur peut injecter la même
implémentation pure pour générer avant insertion, puis utiliser le port d'initialisation dans sa
transaction. Il ne fait pas de requête réseau pendant une transaction longue ni de second générateur.
`History.CurrentAsOf` conserve sa sémantique de date observée ; une interface d'horloge distincte
fournit le dernier instant simulé **validé**, sans consulter l'heure réelle dans le domaine.

Les ports `Pricing.OfferPrice/Rate` existants restent disponibles avec leurs erreurs. Ajouter un port
de gel/lecture de snapshot séparé ; ne pas agrandir automatiquement tous les mocks des autres domaines.
Le contrat de NLPF-69 observé (`FreezePrices`, `SnapshotOfferPrice`, `SnapshotRate`) est un consommateur
à raccorder explicitement, pas une interface déjà adoptée. Ses méthodes de paire exacte restent
exactes ; la résolution de l'inverse appartient au service de conversion.

## Formats communs

- Préfixe `/api/v1`, JSON `snake_case`, erreur `{"error":"message explicite"}` ; détails de manques
  structurés et triés quand pertinents (`missing_offer_ids`, `missing_pairs`), sans remplacer l'erreur.
- Money : `{"amount":"10.125","currency":"USD"}` ; taux, bandes, notes, budgets et pourcentages
  en chaînes décimales finies. Pas de nombre JSON pour une valeur économique, NaN ou Infinity.
- Identifiants et jours : entiers ; timestamps RFC3339 avec fuseau, normalisés UTC. Contexte générateur
  quotidien à minuit UTC ; ni date actuelle ni taux implicite. Sources et devises validées.
- Dernière cotation éligible : date <= as_of, puis ID décroissant en cas d'égalité. Prix et FX d'une
  lecture proviennent du même instantané de lecture, jamais d'un état d'ingestion partiellement validé.
- DTO prix persisté : `{id,offer_id,price,source,quoted_at}`. DTO FX :
  `{id,base_currency,quote_currency,rate,source,quoted_at}`. Avant insertion, la source ne fournit pas
  un ID DB ; clé de cotation = source + série/entité + instant, validée au stockage.
- Historique : `[from,to]` inclus, `from<=to`, pas de cotation future au-delà du contexte validé.
  Ordre date/ID croissant. L'API d'historique brute ne renvoie pas implicitement le point antérieur ;
  le port statistique NLPF-67 conserve son point de référence additionnel et sa fenêtre calendaire.

## API externe priceapi — cinq opérations

Ces routes sont internes à la stack ; `/healthz` reste inchangé. Les POST sont des calculs sans
écriture dans priceapi, aucun effet de bord d'une répétition. Source reçoit les définitions et
l'empreinte du manifeste du back ; elle n'a pas besoin de rappeler le back pour les retrouver.

| Route source | Entrée particulière | Sortie |
|---|---|---|
| POST /quotes/offer | une définition d'offre | cotations de cette offre |
| POST /quotes/supplier | supplier_id et définitions de ses offres | uniquement les offres de ce fournisseur |
| POST /quotes/catalog | définitions des offres demandées | une série complète pour chaque offre |
| POST /fx/pair | une définition de paire | cotations de cette paire exacte |
| POST /fx | définitions des paires demandées | séries de toutes ces paires |

Toutes ces routes sont relatives au préfixe `/api/v1`. Contexte commun :

```json
{
  "generator_version":"pricesim.v1", "dataset_version":"demo-v1", "seed":"42",
  "from":"2026-11-05T00:00:00Z", "to":"2026-11-05T00:00:00Z",
  "manifest_hash":"empreinte-du-manifeste-canonique"
}
```

Définition d'offre : `{offer_id,supplier_id,series_key,currency,price_min,price_max}` avec bornes
en chaînes ; définition FX : `{series_key,base_currency,quote_currency,reference,band_percent}`.
Les enveloppes ajoutent `offer`/`offers` ou `pair`/`pairs` selon l'opération. `from/to` produisent
des jours consécutifs UTC ; le bootstrap couvre 365 jours exactement, le tick un seul jour.

Le manifeste conserve la définition utilisée à chaque période ; hash et source attendus sont
vérifiés dans la réponse. Le client valide l'ensemble exact d'entités/jours demandé : aucun élément
en plus ou manquant, aucune répétition contradictoire. Il vérifie devise, finitude, positifs, bornes,
date et identité avant écriture. Réponses `{generator_version,manifest_hash,quotes:[…]}` ou
`{generator_version,manifest_hash,rates:[…]}`, ordre stable. Les limites de taille imposent un
découpage explicite et vérifié du client, jamais une liste tronquée ; mêmes séries quel que soit le batch.

## API du back — données persistées

Lire une valeur ne demande pas un tirage supplémentaire au simulateur. Ces lectures utilisent les
cotations déjà ingérées et leur contexte. Les routes catalogue/fournisseur de NLPF-67 sont conservées.

| Route | Résultat |
|---|---|
| GET /quotes/offers/{id} | prix de l'offre et contexte |
| GET /quotes/suppliers/{id} | prix des offres de ce seul fournisseur et contexte |
| GET /quotes | prix de toutes les offres cotées du périmètre courant et contexte |
| GET /fx/{base}/{quote} | taux de la paire **exacte**, sans inverse implicite |
| GET /fx | toutes les dernières paires disponibles et contexte |
| GET /offers/{id}/quotes?from=…&to=… | historique brut des prix d'une offre |
| GET /fx/{base}/{quote}/history?from=…&to=… | historique brut de la paire exacte |
| GET /pricing/status | dernier tick/as_of validés, état de la source et diagnostic |
| POST /price-snapshots | créer un snapshot immuable complet, 201 + Location |
| GET /price-snapshots/{id} | en-tête, cotations liées et couverture |
| POST /conversions | montant converti en sortie et provenance du calcul |
| GET /quotes/stream | signal SSE, sans prix dans l'événement |

Routes relatives au préfixe `/api/v1`. Les lectures courantes acceptent `as_of` optionnel ;
absence = instant simulé validé, non `time.Now`. Instant futur refusé, historique explicite inchangé.
Une ressource absente produit 404 ; format/borne/devise/date invalide 400 ; conflit de configuration
ou couverture 409 ; absence d'une source exploitable et d'un repli complet 503. Les erreurs métier
restent nommées en Go (`errors.Is`), sans dépendre des libellés HTTP.

Une réponse groupée conserve les offres sans prix avec une erreur par entrée et un indicateur
de couverture ; elle n'en fabrique pas un à zéro. Un calcul ou un snapshot qui exige une couverture
complète échoue en listant les manques. Une paire exacte absente ne devient pas automatiquement
une paire inverse dans une lecture FX brute.

Les trois routes de prix renvoient `{context, quotes:[{offer_id,quote,error?}], coverage_complete}`,
avec `quote:null` quand la cotation manque. Les routes FX renvoient `{context,rates:[…]}` ; les
historiques sont des tableaux datés. La page fournisseur conserve ses champs existants et ajoute
`context` quand elle est raccordée au flux. En repli, sa cotation courante garde l'ID figé, même
si une autre cotation a été enregistrée plus tard avec le même horodatage.

## Contexte, panne et snapshot

Les nouvelles réponses enveloppées portent `context` : mode `live`, `snapshot` ou
`snapshot_fallback`, `as_of` réellement utilisé, tick validé si applicable, snapshot_id si applicable
et motif de repli. Garder les dates/sources de chaque cotation. Un contexte fallback porte la date du
snapshot ; l'horloge courante peut être fournie séparément, jamais à la place de cette date.

Sélection de repli après timeout : snapshot fermé le plus récent couvrant toutes les offres et
conversions nécessaires ; en cas d'égalité, ID décroissant. Aucun mélange de snapshots par ligne,
aucun repli à des zéros. Valider finitude, identités/devises, as_of et couverture ; sans candidat,
503 explicite. Les historiques bruts peuvent rester consultables même si le calcul courant échoue.
Les prix figés sont relus **par les IDs liés**, jamais reconstruits par « dernier prix à la même date ».

Corps de création proposé :

```json
{"scope":"supplier","scope_id":12,"target_currencies":["EUR"],"as_of":"2026-11-05T00:00:00Z"}
```

`scope` = offer/supplier/bom/catalog ; scope_id requis sauf catalogue, où il est absent. Pour BOM,
la frontière BOM fournit les offres choisies et devises nécessaires, y compris celles des budgets.
Pour les autres périmètres, `target_currencies` est explicite (tableau vide = prix d'origine seulement).
Dédupliquer IDs/devises, résoudre direct puis inverse dans le contexte unique, vérifier tous les
manques avant création atomique. Pas de snapshot partiel. Les taux liés conservent leur paire réelle.
La garde transactionnelle proposée en ADR 0013 bloque tout ajout après commit, sans rouvrir le legacy.

Conversion : entrée `{money,target_currency,as_of?}` ou `{money,target_currency,snapshot_id}`,
sans mélanger deux contextes. Sortie : montant d'origine exact, Money converti arrondi seulement en
sortie, sens `identity/direct/inverse`, cotation FX d'origine et contexte. L'agrégation BOM et le calcul
de volume utilisent les valeurs exactes internes, jamais le montant déjà arrondi de ce endpoint.

## SSE : signal et reprise

Après commit d'un tick complet :

```text
id: 42
event: tick
data: {"tick":42,"as_of":"2026-11-05T00:00:00Z"}

```

`text/event-stream`, `Cache-Control: no-cache`, absence de buffer proxy ; flush et annulation par
contexte. Premier raccordement : état validé courant (bootstrap tick 0) puis signaux. Pour éviter
une perte entre abonnement et première lecture, abonnement installé avant lecture du curseur puis
déduplication des numéros déjà vus. Aucun événement réussi avant fin du bootstrap.

Last-Event-ID : entier non négatif ; valeur invalide/future → 400 avant ouverture du flux.
Si l'ID est antérieur au courant, envoyer le dernier état validé, pas une relecture de tous les jours ;
si égal, attendre le tick suivant. Le frontend relit aussi REST à l'ouverture/reconnexion, comme il
le fait déjà. File bornée à un signal courant par client, remplacement des intermédiaires possible ;
écriture réseau bornée/annulable, client bloqué déconnecté. Ni perte de cotation ni perte de calcul :
les données sont persistées et REST reste la vérité. Arrêt serveur → fermeture propre des abonnements.

Le générateur ne dépend pas de SSE. Le domaine émet un fait « tick validé », l'adapter HTTP le
traduit selon [le standard EventSource](https://html.spec.whatwg.org/multipage/server-sent-events.html#the-last-event-id-header).
Les tests contrôlés sont conservés, puis complétés d'un vrai back/DB/source/SSE dans les trois moteurs.

## Budgets et frontière BOM proposée

Routes futures, **bloquées sur le partage du vrai service BOM**, sans second moteur :

| Route relative /api/v1 | Usage |
|---|---|
| GET/PUT /boms/{id}/cost-targets | Paramètres complets du brouillon, mode et volume de référence |
| GET/PUT/DELETE /boms/{id}/nodes/{node_id}/cost-target | Budget cible optionnel d'un nœud de cette BOM |
| GET /boms/{id}/cost-targets/evaluation | Comparaisons exactes, répartition et volume finançable expliqués |

PUT remplace les paramètres demandés ; champs facultatifs absents/null = absence explicite,
champs requis null refusés. Aucun changement des sémantiques PATCH catalogue existantes.
Paramètres : budgets Money non négatifs ; warning_percent décimal dans [0,100[, défaut "90" ;
mode manual/production_budget ; production_budget et reference_volume positif requis en second mode.
Le volume manuel reste une valeur positive de brouillon, distincte de l'estimation budgétaire.

À la publication future, copier les paramètres présents et les faits nécessaires dans des tables
associées au nouveau snapshot BOM, sans enrichir ni modifier les anciennes publications.
La devise d'origine des budgets reste conservée, conversion à la devise BOM avec le même contexte
de prix/taux ; absence de taux → comparaison indisponible, sans relabel de devise.

Budgets globaux et de sous-systèmes par voiture. Seuils : coût < seuil×budget vert ; de ce seuil
à budget inclus orange ; dépassement rouge ; coût absent indisponible. Budget zéro/coût zéro vert,
budget zéro/coût positif rouge. Pour les allocations imbriquées, comparer les budgets sans ancêtre
budgété au global, puis les descendants au budget parent approprié ; pas de double comptage.

Volume finançable = floor(budget total converti / coût exact par voiture au volume de référence).
Conserver volume de référence, coût, contrats/paliers et cotations/taux utilisés avec le résultat.
Recalculer avec la même référence quand le prix change, jamais en réinjectant le quotient comme
volume de référence. Si la remise au volume déduit diffère, ne pas présenter cette estimation comme
une optimisation du volume maximal ; le contexte de référence reste visible.

Coût nul/manquant : erreur, pas de résultat infini ou zéro fabriqué. Quotient zéro : résultat
« aucune voiture finançable », pas de clamp à 1 et pas d'UPDATE de boms.volume à 0. Quotient hors
capacité du volume entier BOM : erreur nommée, sans débordement. Une publication en mode budget
exige un volume effectif positif représentable ; les paramètres restent modifiables dans le brouillon.

## Configuration et tests d'acceptation

Graine et identifiant de dataset, as_of initial, paires/références/bandes, vitesse et timeout sont
explicites au câblage ; vitesse par défaut 40m/365 jours. Le processus externe existe déjà : pas de
nouveau conteneur/service. Chaque run conserve ses paramètres initiaux et refuse une reprise dont
la graine, version, date initiale ou les références FX divergent. La cadence réelle et le timeout
peuvent être ajustés au redémarrage sans régénérer de cotations ni rattraper le temps d'arrêt.

Tests prévus : Source en httptest avec erreurs et timeout ; génération identique à travers ordre,
batches et reprise ; 365 lignes exactes deux fois/concurremment ; faux temps aux bornes ; repli
snapshot ou erreur ; refus SQL de mutation/ajout ; inverse périodique avant somme ; budgets/volume
et vrai SSE navigateur. Aucun de ces nouveaux comportements n'est présenté comme implémenté ici.
