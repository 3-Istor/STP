# 0040 — Harnais : guides des agents et relecture IA proportionnée au risque
Statut : Proposé — Date : 2026-11-17 — Décideurs : (à compléter par un humain)

## Contexte
`AGENTS.md` faisait 229 lignes : un agent en lit le début et rate les règles du bas. Aucune relecture IA
n'était outillée : chaque agent relisait son propre travail dans le contexte qui l'avait produit. Les humains
relisaient tout avec la même attention, donc rien avec assez. État de l'art (Anthropic, *How the Claude Code
team does code review*, 15/10/2026) : sévérités Important / Nit / Pré-existant, constats filtrés, niveau
calculé sur le changement, 3 tours au plus, l'approbation d'un agent ne vaut jamais celle d'un humain.

**À trancher par l'humain (recommandation : oui pour les trois)** :
1. `AGENTS.md` devient une carte de 104 lignes ; le texte d'origine est déplacé **mot pour mot** dans
   `docs/agents/regles.md` (§1–5) et `docs/agents/travail-avec-agents.md` (§6–16). Coût d'un non : un agent
   continue de rater la fin d'un fichier de 229 lignes.
2. Ajouter `anthropics/claude-code-action` (épinglée par SHA) dans `ai-review.yml`, **inerte sans le secret
   `ANTHROPIC_API_KEY`**. Coût d'un non : la relecture IA reste locale (skill `revue-par-niveau`).
3. Hooks Claude Code qui **bloquent** l'édition d'une migration commitée, d'une ADR acceptée, de
   `docs/features.md`, et les commandes git dangereuses. Coût d'un non : ces interdits restent du texte.

## Options
1. **Ne rien faire** — rien à maintenir ; règles longues, relecture uniforme, interdits non appliqués.
2. **Relecteur unique généraliste** — simple ; mélange sécurité, métier et style, constats bruyants.
3. **Guides courts + 4 relecteurs spécialisés en contexte neuf, déclenchés par le niveau de risque** —
   `REVIEW.md` (échelle à 4 axes et routage vers l'humain), 7 skills de 43 à 62 lignes, hooks qui bloquent,
   tests qui empêchent la doc des agents de dériver (`internal/archtest/docs_test.go`).

## Décision
Option 3 : l'attention humaine va où le niveau et la sévérité l'envoient, et chaque guide est vérifié par un
capteur calculé, comme le reste du code.

## Conséquences
- Facile : un agent (Claude, Codex, Cursor…) trouve la règle en une page ; l'humain lit « où regarder ».
- Difficile : les renvois « AGENTS §N » du code pointent désormais vers `docs/agents/` (dit dans AGENTS.md §9) ;
  4 relecteurs = coût en jetons au niveau 3.
- Retour arrière : remettre l'ancien `AGENTS.md` depuis git (les deux fichiers `docs/agents/` en sont la copie),
  supprimer `.claude/` et `ai-review.yml`.
- Pivot : une règle métier nouvelle (carbone versionné, multi-usine) = une ligne dans `docs/agents/regles.md`,
  une puce dans `relecteur-metier`, un test.
- Méthode d'audit inspirée de `cloudflare/security-audit-skill` (MIT), réécrite et non vendue : l'installer
  telle quelle est une dépendance de plus, à décider à part.
