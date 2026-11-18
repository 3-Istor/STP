// Stop : avant de rendre la main, rappelle à l'agent le niveau de risque de ce qu'il a changé et
// ce que ce niveau exige (relecteurs, humain). Informatif : ne bloque jamais (ADR 0040).
import { execFileSync } from 'node:child_process';
import { readFileSync } from 'node:fs';

const input = JSON.parse(readFileSync(0, 'utf8') || '{}');
if (input.stop_hook_active) process.exit(0); // pas de boucle
const root = process.env.CLAUDE_PROJECT_DIR || process.cwd();
try {
  const out = execFileSync('go', ['run', './cmd/risk', '-json'], { cwd: root, encoding: 'utf8', timeout: 60000, stdio: ['ignore', 'pipe', 'ignore'] });
  const a = JSON.parse(out);
  if (!a.findings?.length || a.level === 0) process.exit(0);
  const names = ['0 Cosmétique', '1 Routine', '2 Sensible', '3 Critique'];
  const reviewers = [[], ['relecteur-code'], ['relecteur-code', 'relecteur-architecture'],
    ['relecteur-code', 'relecteur-architecture', 'relecteur-securite', 'relecteur-metier']][a.level];
  console.log(`Niveau de risque du changement : ${names[a.level]} (${a.findings[0].reason}, ${a.findings[0].path}). ` +
    `Avant de dire « terminé » : make verify LEVEL=pr, skill revue-par-niveau (${reviewers.join(', ')}), livrable §9.`);
} catch { /* pas de git ou de go : rien à dire */ }
process.exit(0);
