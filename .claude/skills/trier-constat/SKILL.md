---
name: trier-constat
description: Trier un constat signalé par un relecteur IA, la CI, gosec, govulncheck, gitleaks, osv-scanner ou npm audit dans BOM Studio : vrai ou faux positif, sévérité, correction ou exception justifiée. À utiliser dès qu'une alerte ou un commentaire de revue arrive.
---

# Trier un constat

Un constat n'est ni ignoré, ni corrigé à l'aveugle : il est **classé, prouvé, puis traité**.

## 1. Reproduire

- Relecteur IA : refaire le scénario décrit ; écrire le test qui le montre. Pas de reproduction possible
  après un essai sérieux → faux positif probable, le dire avec ce qui a été essayé.
- Scanner de dépendances (govulncheck, osv-scanner, npm audit) : le code vulnérable est-il **atteint** ?
  govulncheck le dit (« called ») ; pour npm, chercher l'import dans `web/src/`.
- gosec / golangci-lint : lire la règle (G101, G304…) et le flux réel de la donnée.
- gitleaks : un vrai secret **reste** dans l'historique même supprimé → révoquer d'abord, puis nettoyer.

## 2. Classer (échelle de `REVIEW.md`)

`[Sévérité · domaine · confiance · impact]`. Exemples dans ce dépôt :

| Constat | Classement probable |
|---|---|
| montant converti en `float` pour un affichage | Important · métier · vérifiée · données |
| route sans `MaxBytesReader` qui accepte un arbre illimité | Important · sécurité · probable · utilisateur |
| CVE dans une dépendance de dev jamais livrée | Nit · sécurité · vérifiée · interne |
| erreur avalée dans un code existant voisin du diff | Pré-existant · correction · probable · utilisateur |

## 3. Traiter

| Verdict | Action |
|---|---|
| vrai, introduit par le diff | corriger + test qui échouait avant, dans le même diff |
| vrai, pré-existant | ticket (`.specs/` ou issue) avec le scénario ; ne pas élargir le diff |
| dépendance vulnérable | montée de version (Renovate/Dependabot) ; sinon exception datée |
| faux positif | exception **la plus étroite possible**, avec la raison sur la ligne |

Exception Go : `//nolint:gosec // G304 : chemin issu de git ls-files, pas d'une entrée utilisateur`.
Jamais de `//nolint` sans raison, jamais une règle désactivée dans `.golangci.yml` pour un seul cas.
Une exception de sécurité est une décision humaine (niveau 3) : l'agent la propose, il ne l'accepte pas.

## 4. Ne jamais

- supprimer, sauter ou affaiblir un test pour faire passer la CI (`docs/agents/travail-avec-agents.md` §11) ;
- baisser un seuil (`--audit-level`, couverture, mutation) sans ADR ;
- fermer un constat sans écrire pourquoi.
