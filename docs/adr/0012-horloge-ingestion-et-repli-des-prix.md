# 0012 — Horloge, ingestion atomique et repli des prix
Statut : Accepté — Date : 2026-11-05 — Décideurs : Joe (accord explicite dans le chat)

## Contexte
`docs/features.md:139,145–148,159,163`, AGENTS §2–3 et ADR 0003 : continuité, pas de doublons,
signal après cotation et repli après timeout. Joe a confirmé la reprise au dernier jour validé,
sans rattrapage du temps d'arrêt ; le mécanisme de persistance reste à choisir.

## Options
1. Horloge/curseur en mémoire — peu de stockage ; reprise et concurrence non garanties.
2. Curseur persistant et transaction d'un tick complet — lecture cohérente et reprise exacte ;
   une source invalide bloque le jour, transactions de taille à mesurer.
3. Écritures indépendantes par offre — transactions petites ; prix et FX de jours différents visibles.

## Décision
**Proposition : option 2**, avec paramètres du run, manifestes et journal de ticks dans pricing,
initialisation et progression transactionnelles ; notification après commit uniquement.

## Conséquences
- Verrou de run et revalidation du manifeste catalogue, puis insertion des cotations, liens de
  provenance du tick et curseur dans la même transaction. Réseau hors transaction longue.
  Une clé existante n'est acceptée que si le contenu est identique ; sinon conflit explicite.
- 365 jours initiaux par série, bootstrap reprenable par la transaction appelante du seed. Run,
  graine, configuration incompatible ou contenu divergent : refus sans écrasement ; changement de
  dataset est explicite. Les anciens prix/taux ne sont jamais UPDATE/DELETE.
- Horloge réelle injectée au câblage ; calcul entier des échéances 40 min / 365, sans dérive du
  raccourci « 6,6 s ». Arrêt ou panne : progression gelée ; reprise au prochain jour non validé,
  sans génération du temps indisponible. Pendant une marche saine, 40 min donnent 365 nouveaux jours.
- Source aberrante : lot entier rejeté et cause journalisée. État d'indisponibilité distinct du
  curseur ; aucun tick réussi émis. Les contraintes de finitude s'ajoutent dans une nouvelle migration ;
  vieilles données invalides diagnostiquées, jamais corrigées automatiquement.
- Après timeout, choisir le snapshot fermé le plus récent couvrant **tout** le contexte demandé,
  avec prix/devises/taux cohérents et dates <= dernier instant validé ; pas un snapshot par ligne.
  Réponse `snapshot_fallback`, ID/date/motif explicites. Sans couverture complète, erreur nommée.
- SSE conserve un signal courant par client ; à la reconnexion, rattrapage par dernier état validé
  puis REST, sans rejouer tous les ticks. Last-Event-ID invalide/futur refusé ; client lent déconnecté
  ou signal intermédiaire remplacé, sans bloquer l'ingestion.
- Tests : interruption/concurrence/conflits, faux temps et reprise, rejet sans commit/signal,
  repli complet ou absent, client SSE lent et reconnexion. Retour arrière : stopper le worker, garder
  les données. Hors-ligne et multi-source restent derrière les ports ; aucun cache/service nouveau.
