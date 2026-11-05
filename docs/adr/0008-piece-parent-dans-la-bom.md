# 0008 — Une pièce peut avoir des enfants dans la BOM
Statut : Accepté — Date : 2026-11-03 — Décideurs : Arthur.presle

## Contexte
bom-tree-editor (`docs/features.md` l. 105) : « ajouter une pièce du catalogue sous un nœud ». La spec ne dit pas
si ce nœud peut être lui-même une pièce, et l'exemple l. 116 (« "Roue" a la quantité 4 et contient "Écrou" ») se lit
dans les deux sens. L'ADR 0007 ne règle que le cas d'une pièce feuille ; le calcul de T5 (NLPF-69) rejette pour
l'instant une pièce qui a des enfants (`ErrInvalidTree`). Au cadrage de T6, l'humain a tranché : une pièce s'ajoute
**sous tout nœud**, pièce comprise. Il faut donc une règle de cumul (règle de calcul absente de la spec, AGENTS §7).

## Options
1. **Refuser** une pièce sous une pièce (sous-ensembles seuls comme parents) — + aucune règle nouvelle.
   − contraire au choix de l'humain ; « Roue » devrait être un sous-ensemble, sans prix ni offre propres.
2. **Pièce parent = sa propre contribution + celle de ses enfants** — + modélise une pièce achetée puis équipée
   (roue + écrous) ; reste une somme et un max, comme l'ADR 0007. − deux sens pour le délai d'une pièce (le sien
   seul, ou avec ses enfants) : l'UI doit montrer les deux.
3. **Ignorer la pièce parent** (seuls ses enfants comptent) — − son prix disparaît du total : donnée cachée (AGENTS §3).

## Décision
**Option 2**, qui étend les règles de l'ADR 0007 sans en changer l'ordre :
1. Quantité réelle : inchangée (produit des quantités de la racine au nœud) ; les enfants d'une pièce sont
   multipliés par sa quantité (4 roues × 5 écrous = 20 écrous).
2. Coût d'une pièce = son prix converti × sa quantité réelle **+** somme des coûts de ses enfants.
3. CO2 et masse d'une pièce = sa valeur × sa quantité réelle **+** somme de ses enfants.
4. Délai d'une pièce = max(délai des enfants) + (livraison de son offre + sa production). Sans enfant, on retrouve
   la règle feuille de l'ADR 0007.
5. Erreurs : un prix ou un taux manquant sur la pièce parent **ou** sur un enfant la prive de coût, avec la liste
   des nœuds en défaut (règle 5 de l'ADR 0007) ; CO2 et délai restent calculés.

Exemple : Roue (pièce, qté 4, 100 €, livraison 3 j, production 2 j) contient Écrou (pièce, qté 5, 1 €, délai 30 j)
→ 105 € par roue, 420 € et 20 écrous pour la voiture ; délai de la roue = 30 + (3 + 2) = 35 j.

## Conséquences
- Facile : le parcours post-ordre de `ComputeTotals` reste O(n) ; il suffit d'ajouter la contribution propre de la
  pièce à la somme de ses enfants. Le snapshot (ADR 0006) recopie déjà les attributs de chaque nœud pièce.
- Difficile : supprimer une pièce supprime aussi ses enfants (cascade existante) : l'UI doit le dire à la confirmation.
- Le panneau latéral affiche la contribution propre et le total avec enfants, pour que « 105 € » s'explique.
- Retour arrière : revenir à l'option 1 = remettre le refus dans `ComputeTotals` et dans le service, plus un contrôle
  des brouillons existants qui auraient des pièces parentes.
- Pivot : carbone versionné → la règle 3 lit le facteur figé de chaque pièce, parente ou non ; multi-usine → aucun impact.
