package bom

// Draft : l'état d'un brouillon lu en une seule fois (BOM, arbre, stock, planning, budgets cibles), dans une même
// lecture cohérente de la base (Store.Draft). Tout ce qui doit être cohérent (contrôles avant publication,
// nœuds figés, faits de coût, % du budget) part de la même valeur : une écriture concurrente entre deux lectures
// ne peut plus produire un snapshot dont les morceaux viennent de deux états différents (v2-lot-g T1, ADR 0038).
type Draft struct {
	BOM      BOM
	Nodes    []Node // triés par id
	Stock    []PartStock
	Schedule *Schedule // nil : pas de planning
	Budgets  []TargetBudget
}

// RootID : id de la racine (parent nil) d'un brouillon ; 0 si aucune.
func RootID(nodes []Node) int64 {
	for _, n := range nodes {
		if n.ParentID == nil {
			return n.ID
		}
	}
	return 0
}
