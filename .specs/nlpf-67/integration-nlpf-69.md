# Intégration NLPF-69 : impact à valider

Inspection en lecture seule du commit `ff5cc69e3e7b9c12622f4ed2c0d230a6a03da2e3`
de la branche distante `nlpf-69`, auteur Arthur.presle, le 2026-11-05.
Aucun checkout, merge, cherry-pick, push ou changement de cette branche. Son état annoncé
dans ses documents n'est pas une validation de son code par NLPF-67.

## Conflits constatés

| Sujet | NLPF-67 | NLPF-69 inspectée | Travail nécessaire avant intégration |
|---|---|---|---|
| Migrations | `00002_catalogue_fournisseurs.sql` | `00002_bom_snapshots.sql` | Deux migrations de même version : pas de concaténation directe. Valider une procédure coordonnée et la tester sur bases jetables aux états v1, v2 catalogue et v2 BOM. Ne pas renuméroter une migration déjà appliquée ni réinitialiser une base utilisateur. |
| ADR | 0006 catalogue, 0007 remises, 0008 statistiques, acceptées par Joe | 0006 snapshots BOM, 0007 totaux, 0008 pièces parentes, acceptées par Arthur | Résoudre les références ambiguës avec l'équipe. Garder le contenu et les décisions acceptées ; documenter toute succession par une nouvelle ADR. |
| Catalogue | Attributs riches et décimaux optionnels pour le legacy : `CO2KGPerUnit`, `MassKG`, `*int` | `CO2Kg`, `MassKg`, `ProductionDays` non optionnels | Adapter les consommateurs. Une masse/émission absente doit produire une erreur nommée, jamais un zéro. |
| BOM et archivage | Protocole pièce puis offre, `FOR SHARE`, validation des archives | Ajouts/choix d'offre sans ce protocole catalogue | Partager les verrous avec archivage/suppression et ajouter le test de concurrence avant raccordement. |
| Remises et snapshots | Évaluateur supplier précis, sélection/date/quantité explicites | Totaux et snapshots sans choix de contrat ni remise figée | Valider le modèle de sélection et les faits contractuels à figer, puis intégrer l'évaluateur ; une évolution du modèle partagé exige une ADR et une nouvelle migration. |
| Prix | Lecture d'historique persisté et statistiques | Interfaces de gel/lecture snapshot ; démo avec prix/taux contrôlés | Raccorder l'implémentation de NLPF-68, son horloge, sa conversion et son flux. La démo ne constitue pas le flux live. |
| Câblage et UI | Services Catalog/Supplier/History et espace fournisseurs | Services BOM/Snapshot et éditeur dans les mêmes fichiers serveur/App/CSS | Composer routes et navigation sur une branche d'intégration autorisée ; conserver les tests des deux parcours. |
| Seed | Contrat déterministe ADR 0009 ; pas de commande complète | Démo à identifiants réservés et petit catalogue, pas de seed Multipla complet | Réutiliser le générateur d'historique de NLPF-68 et les services BOM, ajouter manifeste/identités/transaction ; vérifier répétition et ouverture réelle des ~30 000 nœuds. |

Le journal NLPF-69 mentionne des snapshots publiés #207/#208 dans la base de développement.
Cette inspection n'a accédé à aucune base de l'équipe. Aucune migration/reset/purge ne doit y être
tentée pour résoudre ces conflits.

## Options soumises à l'équipe

1. Attendre la livraison coordonnée de NLPF-68/69 dans une base commune : conserver NLPF-67
   et ses preuves locales, puis reprendre T7–T10 sur ces dépendances disponibles.
2. Avec l'accord d'Arthur, préparer une branche d'intégration distincte des branches individuelles,
   puis soumettre les décisions de migrations/ADR et de snapshots contractuels avant leur code.
   Tester les migrations avec des fixtures de chaque historique, les snapshots conservés, puis
   les remises BOM, le flux réel et le seed complet. Toute fusion reste une action séparée.

Les accords antérieurs de Joe portent sur les ADR 0006–0010 de NLPF-67 ; ils ne résolvent pas
ces conflits interbranches ni les nouveaux choix de modèle. AGENTS.md §6–8, §11 et §13 s'appliquent.

## Conditions de reprise

- Accord de l'humain propriétaire pour utiliser les travaux NLPF-69 et partage des fichiers.
- Procédure de migration et traçabilité des ADR acceptées explicitement validées.
- Contrat de sélection/gel des remises validé pour les vrais totaux et snapshots.
- Livraison NLPF-68 : ingestion/historique, horloge, SSE et générateur transactionnel du seed.

Sans ces éléments, T7 n'a pas sa preuve de SSE réel, T8 n'a pas son moteur raccordé et T9 n'a
ni historique initial ni éditeur intégré. Les remplacer par des stubs ne satisfait pas la spec.
