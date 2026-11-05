# 0016 — Fermeture transactionnelle des snapshots de prix
Statut : Accepté — Date : 2026-11-05 — Décideurs : Joe (accord explicite dans le chat)

## Contexte
ADR 0002 et `docs/features.md:140,156–158` exigent un snapshot immuable. Les triggers existants
refusent UPDATE/DELETE/TRUNCATE, mais autorisent un INSERT tardif dans ses tables de liens.
Les snapshots publiés existants ne doivent subir aucune modification de prix, taux ou périmètre.

## Options
1. Service seul interdisant les ajouts — pas de migration ; SQL direct contourne l'invariant.
2. Autoriser les liens uniquement dans la transaction créant l'en-tête — fermeture implicite
   au commit, sans seconde phase ; métadonnée transactionnelle PostgreSQL supplémentaire.
3. État ouvert/fermé — explicite ; exception d'UPDATE à concevoir et états incomplets à empêcher.

## Décision
**Proposition : option 2** : ajouter `creation_xid xid8` nullable à l'en-tête, renseigné
automatiquement par trigger à l'INSERT ; un lien ne s'insère que si cet identifiant égale
`pg_current_xact_id()`. La fermeture est le commit, sans réouverture possible.

## Conséquences
- Nouvelle migration additive ; les anciennes lignes gardent NULL et sont donc déjà fermées.
  Aucun UPDATE des faits existants, aucune reconstruction ni renumérotation de snapshot.
- Le trigger d'en-tête impose la vraie transaction, sans accepter une valeur fournie par un client.
  Les triggers de liens vérifient l'en-tête visible et son identifiant ; nouvelle transaction,
  en-tête absent ou NULL → refus. Les protections antérieures restent en place.
- Unité de création : lire au même instant prix et taux nécessaires, vérifier toute la couverture,
  insérer en-tête et liens, commit. Un échec annule tout. Le service ne propose aucune édition.
- En PostgreSQL, la fonction renvoie l'identifiant de la transaction principale même depuis un
  savepoint, et xid8 évite la réutilisation du compteur xid32. Le mécanisme reste interne au store,
  sans exposer un identifiant transactionnel dans les calculs ou l'API.
- Tests obligatoires : liens dans la transaction initiale acceptés ; ajout après commit refusé,
  y compris dans une autre connexion concurrente ; valeur forgée refusée, legacy fermé et identités/
  prix/taux inchangés. Des tests restent à écrire : cette ADR n'est pas une preuve d'implémentation.
- Retour arrière : arrêter la création des snapshots et conserver garde/métadonnée ; ne pas
  rouvrir les objets existants. Un down/reset ne sert qu'aux bases de test jetables. Hors-ligne et
  multi-source : les cotations liées demeurent lisibles ; aucun modèle JSONB ou service supplémentaire.

Référence : [PostgreSQL 18, identifiants de transaction](https://www.postgresql.org/docs/18/functions-info.html#FUNCTIONS-PG-SNAPSHOT),
consultée le 2026-11-05.
