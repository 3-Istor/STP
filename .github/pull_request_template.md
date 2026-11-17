<!-- Titre : <type>(<feature>): <intention>  ex. feat(bom-tree-editor): déplacer un nœud sans cycle -->

Closes #

## Niveau de risque
<!-- Sortie de `make risk` (la CI la recalcule, job risk). Niveau ≥ 2 : lien vers le plan validé. -->
Niveau : _ — Plan validé par : _

## Où l'humain doit regarder
<!-- Dernière ligne du résumé de la skill revue-par-niveau : fichiers et lignes à relire en priorité. -->

## Ce que fait le changement
<!-- Une phrase, puis la responsabilité de chaque fichier touché (AGENTS §9.1). -->

## Critères d'acceptation
<!-- Recopiés de docs/features.md ; chaque ligne → le test qui la prouve. -->
- [ ] QUAND … ALORS … DOIT … — `TestXxx/nom du cas`

## Cas de panne prévisible et test qui le couvre
<!-- AGENTS §9.2 et §9.3 -->

## Non fait / non vérifié
<!-- AGENTS §9.4 — dit honnêtement. -->

## Checklist (Definition of Done)
- [ ] `make verify LEVEL=pr` vert (étapes `not_run` listées ci-dessous)
- [ ] Relecture IA selon le niveau (REVIEW.md) : Important corrigés, Pré-existant en ticket, ≤ 3 tours
- [ ] ADR référencée si une décision §7 a été prise : ADR ____
- [ ] Pas de `float` pour un montant, pas de logique métier dans un handler
- [ ] Alertes de sécurité traitées ou acceptées avec raison
- [ ] Trailer `Co-Authored-By:` sur les commits écrits avec une IA
