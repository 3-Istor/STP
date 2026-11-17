// Tables de correspondance versionnées (ADR 0035). Seul point d'import des JSON de données.
import slotsJson from './data/slots.json' with { type: 'json' }
import type { SlotTable } from './mapping.ts'

export const slotTable = slotsJson as unknown as SlotTable
