---
name: ajouter-migration
description: Ajouter une migration goose PostgreSQL à BOM Studio (table, colonne, index, trigger) en restant réversible et sans toucher aux snapshots publiés. À utiliser pour tout changement de schéma ou de requête sqlc.
---

# Ajouter une migration

## Règles

- **Jamais** modifier une migration déjà commitée : on en ajoute une nouvelle (`docs/agents/regles.md` §2).
- Numéro suivant dans `migrations/` (`000NN_nom_en_snake.sql`), une intention par fichier.
- `-- +goose Up` et `-- +goose Down` tous les deux ; le `Down` remet le schéma précédent.
- Montant = `NUMERIC` + devise `CHAR(3)`, jamais `REAL`/`DOUBLE PRECISION`.
- Les tables de snapshot sont figées par trigger (`reject_mutation`, ADR 0038) : une migration n'y fait
  ni `UPDATE` ni `DELETE`. Ajouter une colonne : `ADD COLUMN … DEFAULT` (ne déclenche pas les triggers).
- Fonctions et triggers entre `-- +goose StatementBegin` / `StatementEnd`.
- Une en-tête en commentaire dit **pourquoi** et cite l'ADR ou le ticket.
- Table partagée entre domaines ou modèle des snapshots changé = décision d'architecture : ADR d'abord.

## Étapes

1. Écrire la migration ; si des requêtes changent, modifier `internal/<domaine>/queries.sql` (voir `sqlc.yaml`) puis `go tool sqlc generate`
   (le code généré est commité ; `go tool sqlc diff` doit être vide).
2. Vérifier la réversibilité sur une base locale :
   ```sh
   make db
   go tool goose -dir migrations postgres "$DATABASE_URL" up
   go tool goose -dir migrations postgres "$DATABASE_URL" reset
   go tool goose -dir migrations postgres "$DATABASE_URL" up
   ```
   La CI rejoue exactement cette séquence.
3. Test d'intégration dans le `*store` concerné (`DATABASE_URL` requis, sinon le test saute — pas vert).
   Pour un trigger : un test qui tente l'opération interdite et attend l'erreur.
4. Clé étrangère : index sur la colonne qui référence. Contrainte `CHECK` pour une règle simple
   (quantité > 0, devise sur 3 lettres).
5. Jeu de démo (`make seed`) toujours rejouable : relancer et vérifier qu'il est idempotent.
6. `make verify LEVEL=pr` ; skill `revue-par-niveau` (niveau 2 au minimum, 3 si snapshot).

## Cas de panne à couvrir

- migration appliquée sur une base qui contient déjà des données de démo ;
- `Down` qui échoue parce qu'une colonne ajoutée est utilisée par un trigger ;
- trigger d'immuabilité contourné par un `INSERT` tardif (ADR 0038).
