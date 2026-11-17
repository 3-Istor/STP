// Rendu Réaliste : livré par la tâche T5 (voir ADR 0035). Tant qu'il n'est pas là, repli sur le rendu Simple.
import type { RealisticLoader } from './viewer.ts'

export const loadRealistic: RealisticLoader = () =>
  Promise.reject(new Error('Rendu réaliste pas encore disponible : rendu Simple affiché.'))
