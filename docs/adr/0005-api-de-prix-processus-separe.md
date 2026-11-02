# 0005 — API de prix en processus séparé, toute l'app lancée par un seul `docker compose`
Statut : Proposé — Date : 2026-11-02 — Décideurs : <à compléter>

## Contexte
`compose.yaml` ne lance que Postgres ; back et front tournent sur le poste (`make dev`, Go + Node dans WSL).
On veut un seul `docker compose up` pour la démo / soutenance et pour un membre sans toolchain. L'API de prix
de live-price-feed (`docs/features.md` l. 124-161) est « externe (simulée pour l'instant) » et AGENTS §2 impose
qu'elle passe par un adapter (timeout → dernier snapshot, prix aberrant → rejet). Où elle tourne est une
frontière, et ajouter des services est une décision du §7. Les choix ci-dessous ont été validés à l'oral
par l'humain au cadrage (ticket `docker-compose-stack`), reste à les acter.

## Options
1. **Ne rien faire** — compose = Postgres seul, simulateur futur dans le back. + rien à maintenir.
   − pas de démo en une commande ; la frontière « externe » reste théorique (timeouts jamais exercés).
2. **Simulateur dans le processus du back** (goroutine derrière l'interface `Pricing`) + compose
   db/back/front. + un binaire de moins, pas de contrat HTTP. − la panne réseau, la latence et le
   redémarrage indépendant de l'API ne se testent pas ; le pivot « plusieurs sources de prix » exigera
   de toute façon de sortir le simulateur.
3. **Binaire `cmd/priceapi` dans un conteneur dédié**, appelé plus tard en HTTP par un adapter du back ;
   compose complet : `db`, `migrate` (one-shot), `back`, `priceapi`, `web`. + frontière réelle : on peut
   couper, ralentir ou redémarrer l'API et voir le fallback ; une autre source = un autre conteneur derrière
   le même adapter. − un binaire, une image et un contrat HTTP de plus à maintenir.

## Décision
**Option 3.** Elle rend testable ce qu'AGENTS §2 exige face à l'API de prix (timeout, rejet, version
utilisée) et prépare le pivot multi-source sans le coder. Détails retenus :
- une image Go multi-stage (`Dockerfile` racine) avec trois cibles : `server`, `priceapi` (alpine, non root,
  `wget` pour le healthcheck) et `migrate` (`go tool goose … up`, pas de dépendance nouvelle) ;
- front : build Vite servi par nginx, qui proxifie `/api` et `/healthz` vers le back, sans bufferiser
  (SSE, ADR 0003) ;
- `migrate` one-shot : `back` attend `service_completed_successfully`, une migration en échec bloque le back ;
- images de base épinglées (tag précis, jamais `latest`) ; front exposé sur `:8000`, back et API de prix non
  exposés sur l'hôte, pour ne pas gêner `make dev` (8080/5173).

## Conséquences
- Facile : `docker compose up --build --wait` démarre tout ; un job CI `compose` fait un smoke test ;
  l'adapter HTTP de live-price-feed aura un vrai service distant à appeler.
- Difficile : deux binaires à faire évoluer ensemble (contrat HTTP à versionner, `/api/v1` côté priceapi) ;
  les builds Docker allongent la CI ; pas de hot reload en Docker (`make dev` reste l'outil de dev).
- Ce ticket ne pose que le squelette (`GET /healthz`) et passe `PRICE_API_URL` au back sans qu'il la lise.
- Retour arrière : retirer `priceapi` de `compose.yaml` et brancher le simulateur en interne derrière
  `Pricing` (option 2) ; le domaine ne change pas, seul l'adapter disparaît.
- Pivot : hors-ligne → l'adapter retombe sur le dernier snapshot, à tester en coupant le conteneur ;
  multi-source → un conteneur par source ; carbone versionné et multi-usine → sans impact sur cette stack.
