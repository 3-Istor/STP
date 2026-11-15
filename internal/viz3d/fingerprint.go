package viz3d

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strconv"
	"strings"

	"bomstudio/internal/bom"
)

// Fingerprint résume l'arbre d'une BOM en une empreinte stable : SHA-256 de la liste des nœuds triée par
// id (id, parent, pièce, quantité). Même arbre = même empreinte, quel que soit l'ordre reçu ; tout ajout,
// retrait, déplacement ou changement de pièce ou de quantité la change. Elle sert à dire qu'une génération
// est « stale » : l'offre, la stratégie ou les jours de production n'y entrent pas, la scène n'en dépend pas.
func Fingerprint(nodes []bom.Node) string {
	sorted := slices.Clone(nodes)
	slices.SortFunc(sorted, func(a, b bom.Node) int { return cmp.Compare(a.ID, b.ID) })
	var sb strings.Builder
	for _, n := range sorted {
		sb.WriteString(strconv.FormatInt(n.ID, 10))
		sb.WriteByte('|')
		if n.ParentID != nil {
			sb.WriteString(strconv.FormatInt(*n.ParentID, 10))
		}
		sb.WriteByte('|')
		if n.PartID != nil {
			sb.WriteString(strconv.FormatInt(*n.PartID, 10))
		}
		sb.WriteByte('|')
		sb.WriteString(n.Quantity.String())
		sb.WriteByte('\n')
	}
	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:])
}
