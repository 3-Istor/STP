// PreToolUse (Bash) : complète les refus de settings.json pour les formes que les motifs ratent
// (commandes chaînées, options dans le désordre). Code 2 = bloqué, message renvoyé à l'agent.
import { readFileSync } from 'node:fs';

const input = JSON.parse(readFileSync(0, 'utf8') || '{}');
const cmd = String(input.tool_input?.command || '');
const rules = [
  [/\bgit\s+push\b[^;&|]*(\s--force\b|\s-f\b|\s\+\S)/, 'push forcé interdit (réécriture d\'historique).'],
  [/\bgit\s+push\b[^;&|]*\s(main|HEAD:main|refs\/heads\/main)\b/, 'push vers main interdit : passer par une PR relue par un humain.'],
  [/\bgit\s+commit\b[^;&|]*\s(--no-verify|-n)\b/, 'commit sans hooks interdit : la moulinette rapide doit passer.'],
  [/\bgit\s+(rebase|filter-branch|filter-repo)\b/, 'réécriture d\'historique : accord humain explicite requis.'],
  [/\bgoose\b[^;&|]*\b(down|reset)\b(?![^;&|]*localhost|[^;&|]*127\.0\.0\.1)/, 'goose down/reset hors base locale.'],
  [/\b(DELETE\s+FROM|UPDATE|TRUNCATE)\s+bom_snapshot/i, 'données de snapshot publiées : on n\'y touche jamais (AGENTS.md §5).'],
  [/\bnpm\s+(install|i)\b/, 'utiliser npm ci ; ajouter une dépendance demande un accord humain.'],
  [/\bgo\s+get\b/, 'ajouter une dépendance Go demande un accord humain et une ADR.'],
];
for (const [re, why] of rules) {
  if (re.test(cmd)) {
    process.stderr.write(`Bloqué par .claude/hooks/garde-commandes.mjs : ${why}\nCommande : ${cmd}\n`);
    process.exit(2);
  }
}
process.exit(0);
