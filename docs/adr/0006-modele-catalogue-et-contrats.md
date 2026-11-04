# 0006 — Modèle du catalogue et des contrats fournisseurs
Statut : Proposé — Date : 2026-11-04 — Décideurs : à renseigner par l'humain

## Contexte
NLPF-67 étend les tables partagées `parts`, `offers`, `suppliers` du socle NLPF-70
(`docs/features.md:51–57`, `228–231`). Leurs FK sont déjà consommées par BOM et pricing.
Il faut préserver les données existantes et les cotations immuables. Échelles confirmées : notes
de 0 à 5 ; matière recyclée et recyclabilité de 0 à 100 %, tous en décimal.

## Options
1. **Étendre les tables et garder les domaines existants** — FK et lectures actuelles conservées ;
   migration coordonnée requise, données anciennes parfois incomplètes.
2. **Ajouter des tables de détails et un service catalogue/fournisseurs commun** — tables initiales
   stables ; jointures supplémentaires et nouvelle frontière métier à maintenir.
3. **Ne rien faire** — pas de migration ; les attributs, contrats et archivages du ticket manquent.

## Décision
**Proposition : option 1**, car le socle prévoit ces attributs et la répartition par domaine existe.
`catalog` possède pièces/offres/alternatives ; `supplier` possède fournisseurs/contrats/paliers ;
`pricing` possède les cotations/statistiques (ADR 0002). La composition des lectures est câblée
dans l'application ; aucune dépendance cyclique. Garder `catalog.Catalog` et `pricing.Pricing`
compatibles ; BOM définit son besoin de remise, l'implémentation supplier y est injectée.

## Conséquences
- Ajouter masse, CO2, pourcentages, coûts/devises, heures, taux horaire/devise, délai aux pièces ;
  qualité aux offres, qualité/confiance aux fournisseurs et `archived_at` aux pièces/offres.
  NUMERIC pour tous les décimaux, montants/devises par paire ; heures/masse/CO2/coûts/délais ≥ 0.
- Nouvelle migration uniquement. Attributs absents des anciennes lignes nullable, jamais zéro
  inventé ; les nouveaux enregistrements sont complets et un calcul incomplet renvoie une erreur.
  Une paire montant/devise est soit complète, soit entièrement absente sur une ligne ancienne.
- Alternative = une paire canonique `(part_low_id, part_high_id)`, `low < high`, unique ; lecture
  dans les deux sens. Deux offres rattachent le même `part_id` ; aucune fusion automatique par nom.
- Contrats datés et conditions textuelles informatives, liens contrat–offre limités au même
  fournisseur, paliers à seuil strictement positif unique et remise [0,100] %. Choisir le seuil
  maximal atteint ; aucune obligation d'augmentation du taux. La sélection relève du brouillon BOM.
- Archiver si pièce/offre référencée par BOM ou offre par cotation/contrat ; masquer également les
  offres d'une pièce archivée dans les listes. Garder la lecture par ID ; refuser leur nouvel ajout
  à un brouillon. Suppression physique seulement sans référence, sans cascade sur l'historique.
  Verrouillage transactionnel + FK RESTRICT ; un conflit concurrent entraîne reprise contrôlée
  vers archivage. Ajout BOM : lire/vérifier pièce puis offre sous FOR SHARE ; suppression/archivage :
  même ordre sous FOR UPDATE. Le FOR KEY SHARE implicite des FK ne bloque pas archived_at (NLPF-69).
- Retour arrière : migration explicite avec export des nouveaux attributs, jamais réécriture de
  00001. Hors-ligne, multi-source et carbone versionné restent hors périmètre ; une future version
  carbone demandera une nouvelle ADR. Les montants conservent déjà leur devise ; pas de multi-usine.
