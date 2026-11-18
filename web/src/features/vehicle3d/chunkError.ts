// Message d'échec d'un import dynamique : Chrome, Firefox, Safari (chunk supprimé par un redéploiement).
export const isStaleChunkError = (message: string) =>
  /dynamically imported module|Importing a module script failed/i.test(message)
