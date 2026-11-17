// PreToolUse (Edit/Write) : refuse ce que AGENTS.md §5 interdit sans accord, avant que le fichier change.
// Code de sortie 2 = l'outil est bloqué et le message est renvoyé à l'agent (ADR 0040).
import { execFileSync } from 'node:child_process';
import { readFileSync, existsSync } from 'node:fs';
import { relative, resolve } from 'node:path';

const input = JSON.parse(readFileSync(0, 'utf8') || '{}');
const root = process.env.CLAUDE_PROJECT_DIR || process.cwd();
const target = input.tool_input?.file_path || input.tool_input?.notebook_path;
if (!target) process.exit(0);
const rel = relative(root, resolve(root, target)).split('\\').join('/');

const committed = (p) => {
  try { execFileSync('git', ['cat-file', '-e', `HEAD:${p}`], { cwd: root, stdio: 'ignore' }); return true; }
  catch { return false; }
};
const block = (why) => { process.stderr.write(`Bloqué par .claude/hooks/garde-fichiers.mjs : ${why}\n`); process.exit(2); };

if (/^migrations\/\d+_.*\.sql$/.test(rel) && committed(rel)) {
  block(`${rel} est une migration déjà commitée : en ajouter une nouvelle (skill ajouter-migration).`);
}
if (/^docs\/adr\/\d{4}-.*\.md$/.test(rel) && existsSync(resolve(root, rel))) {
  const text = readFileSync(resolve(root, rel), 'utf8');
  if (/^Statut\s*:\s*Accept/m.test(text)) {
    block(`${rel} est une ADR acceptée : on ne la réécrit pas, on en crée une qui la remplace (skill rediger-adr).`);
  }
}
if (rel === 'docs/features.md') block('docs/features.md ne se modifie qu\'avec l\'accord explicite d\'un humain (AGENTS.md §5).');
if (/(^|\/)\.env(\.|$)/.test(rel)) block('fichier de secrets : jamais lu ni écrit par un agent.');
process.exit(0);
