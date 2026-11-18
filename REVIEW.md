# REVIEW.md — comment on relit, et ce qui mérite un humain

Lu par les relecteurs IA (`.claude/agents/`, revue Claude sur PR) et par les humains. But : que
l'humain ne lise que ce qui compte (ADR 0040, Anthropic « How the Claude Code team does code review », 2026).

## Ce qu'on ne signale pas

Ce qu'un capteur calculé vérifie déjà : format (gofmt, oxlint), imports interdits (`internal/archtest`),
routes retirées (`TestRoutesContract`), failles connues (govulncheck, npm audit), secrets (gitleaks).
Pas de goût personnel, pas de « on pourrait aussi », pas de couverture pour la couverture.

## Chaque constat est noté sur quatre axes

| Axe | Valeurs |
|---|---|
| **Sévérité** | `Important` : à corriger avant merge (bug, règle de `docs/agents/regles.md` §3 violée, faille exploitable, contrat cassé) · `Nit` : mineur, ne bloque pas · `Pré-existant` : vrai problème, pas introduit par ce diff |
| **Domaine** | `métier` (montants, arbre, snapshots, reproductibilité) · `sécurité` · `contrat` (`/api/v1`, schéma) · `architecture` · `correction` · `tests` |
| **Confiance** | `vérifiée` : reproduite (test qui échoue, commande, trace) · `probable` : chemin d'exécution précis cité · `spéculative` : « pourrait » sans chemin |
| **Impact** | `données` (montant faux, snapshot altéré, fuite) · `utilisateur` (écran faux, erreur) · `interne` (maintenance) |

Format d'un constat (une entrée par problème, la classe de bug une seule fois avec toutes ses occurrences) :

```
[Important · métier · vérifiée · données] internal/bom/cost.go:142
Ce qui casse : deux devises additionnées sans conversion quand l'offre change de devise.
Scénario : BOM EUR, offre passe en USD → total faux, aucune erreur.
Preuve : TestCostMixedCurrency (ajouté, échoue sur ce commit).
Correction proposée : passer par money.Exact.Exchange avec le taux daté.
```

## Ce qui remonte à l'humain (routage)

| Constat | Destination |
|---|---|
| `Important` + confiance `vérifiée` ou `probable` | **bloque** : corrigé ou fermé avec une raison écrite par un humain |
| `Important` + `spéculative` | l'agent cherche une preuve ; sans preuve après un tour, il le jette |
| `Nit` | une ligne groupée dans le résumé, jamais une question |
| `Pré-existant` | ticket (`.specs/` ou issue), ne bloque pas ce diff |
| domaine `métier` ou `sécurité`, niveau de risque ≥ 2 | toujours lu par un humain, même en `Nit` |

## Convergence

- **3 tours de revue au plus.** Au 4ᵉ, l'agent arrête et propose de revoir l'approche.
- Après le 1ᵉʳ tour, on ne relit que le nouveau diff ; un constat déjà publié n'est pas répété.
- Une classe de bug signalée est corrigée **partout**, pas seulement à la ligne citée.
- L'approbation d'un agent ne compte jamais comme celle d'un humain ; un nouveau push d'agent
  remet l'approbation humaine à zéro.
- « Aucun constat » est une réponse valable et attendue.
