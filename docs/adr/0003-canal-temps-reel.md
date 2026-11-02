# 0003 — Canal temps réel : comment l'UI apprend qu'il y a de nouvelles cotations
Statut : Accepté — Date : 2026-11-02 — Décideurs : Arthur.presle

## Contexte
live-price-feed (`docs/features.md`) : « les clients (UI) sont notifiés des nouvelles cotations ». Le
simulateur produit une journée simulée toutes les ~6,6 s (vitesse paramétrable) ; à chaque tick, toutes les
offres et toutes les paires reçoivent une cotation. Un brouillon « en direct » doit alors recalculer ses
totaux (< 1 s pour 30 000 nœuds). Contraintes : mono-utilisateur, pas d'authentification, front Vite avec
proxy `/api` → `:8080`, stdlib d'abord (AGENTS §2). Choix transverse du §7 (temps réel).

## Options
1. **Ne rien faire / rechargement manuel** — zéro code ; ne respecte pas l'exigence « notifiés ».
2. **Polling** — l'UI interroge `GET /api/v1/quotes/latest?since=…` toutes les N s.
   + trivial, sans état serveur, rejouable avec `curl`, se comporte bien hors-ligne (l'appel échoue, on réessaie).
   − latence jusqu'à N s ; requêtes inutiles quand rien ne change ; N à régler avec la vitesse du simulateur.
3. **Server-Sent Events (SSE)** — `GET /api/v1/quotes/stream`, `text/event-stream`, côté navigateur
   `EventSource`.
   + stdlib seule (`http.ResponseController.Flush`), aucune dépendance npm ; reconnexion automatique et
   reprise par `Last-Event-ID` natives ; unidirectionnel, ce qui suffit ; passe par le proxy Vite.
   − une connexion ouverte par onglet ; `WriteTimeout` du serveur à exclure pour cette route ; un tampon
   par client à borner (client lent).
4. **WebSocket** — bidirectionnel.
   + latence minimale. − dépendance Go (`coder/websocket` ou équivalent) ; protocole à concevoir
   (ping, reconnexion, reprise) ; le sens client → serveur n'a pas d'usage ici.

## Décision
**Option 3, SSE, comme simple signal**. Chaque évènement porte l'horloge simulée et un numéro de
tick (`id:`), pas les prix : l'UI relit ensuite les totaux par l'API REST habituelle. Le flux reste petit
(un évènement par tick), le calcul reste unique côté Go, et l'API REST demeure la seule source de vérité.

## Conséquences
- Facile : ajouter d'autres types d'évènements (`event: snapshot_created`) sans changer le transport.
- Facile : repli sur l'option 2 si SSE pose problème — le client fait le même appel REST, seul le
  déclencheur change (minuterie au lieu d'évènement). Retour arrière = supprimer la route.
- Difficile : tests du flux (lecture incrémentale d'une réponse `httptest`) ; à ne pas mettre derrière un
  proxy qui tamponne (en-tête `X-Accel-Buffering: no` si nginx un jour).
- Le domaine ne dépend pas du canal : `pricing` publie « tick N terminé » sur une interface, l'adapter HTTP
  le traduit en SSE (AGENTS §2).
- Pivot hors-ligne strict : `EventSource` se reconnecte seul au retour du réseau, et le client relit l'état
  courant ; aucune donnée n'est perdue puisque l'évènement n'est qu'un signal. Multi-source de prix : un
  évènement par source ou un champ `source`, sans changer le transport. Multi-usine : filtrage par paramètre
  de requête sur la route.
