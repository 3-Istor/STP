package httpapi

import (
	"flag"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

var updateRoutes = flag.Bool("update-routes", false, "réécrit testdata/routes.golden (ajout de route uniquement, voir skill contrat-api)")

// declaredRoutes lit, dans le code source, chaque motif passé à mux.HandleFunc / mux.Handle.
func declaredRoutes(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("analyse de %s : %v", name, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || (sel.Sel.Name != "HandleFunc" && sel.Sel.Name != "Handle") {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			if v, err := strconv.Unquote(lit.Value); err == nil {
				seen[v] = true
			}
			return true
		})
	}
	routes := make([]string, 0, len(seen))
	for r := range seen {
		routes = append(routes, r)
	}
	sort.Strings(routes)
	return routes
}

// TestRoutesContract fige l'inventaire des routes /api/v1 (capteur calculé, ADR 0039).
// Une route ajoutée : on régénère avec -update-routes et le diff du golden est relu (niveau 2).
// Une route retirée ou renommée casse les clients : interdit sans ADR (AGENTS §7), le test échoue
// même avec -update-routes pour qu'aucun agent ne l'efface en régénérant.
func TestRoutesContract(t *testing.T) {
	const golden = "testdata/routes.golden"
	got := declaredRoutes(t)
	if len(got) < 20 {
		t.Fatalf("seulement %d routes lues : l'analyse ne voit plus les déclarations", len(got))
	}
	data, err := os.ReadFile(golden)
	if err != nil && !*updateRoutes {
		t.Fatalf("lecture %s : %v (créer avec go test ./internal/httpapi -run TestRoutesContract -update-routes)", golden, err)
	}
	var want []string
	for _, l := range strings.Split(string(data), "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
			want = append(want, l)
		}
	}
	gotSet := map[string]bool{}
	for _, r := range got {
		gotSet[r] = true
	}
	removed := 0
	for _, r := range want {
		if !gotSet[r] {
			removed++
			t.Errorf("route retirée ou renommée : %q — changement non rétrocompatible de /api/v1, ADR requise (AGENTS §7)", r)
		}
	}
	wantSet := map[string]bool{}
	for _, r := range want {
		wantSet[r] = true
	}
	var added []string
	for _, r := range got {
		if !wantSet[r] {
			added = append(added, r)
		}
	}
	if len(added) == 0 {
		return
	}
	if !*updateRoutes || removed > 0 {
		t.Errorf("routes nouvelles non inscrites au contrat : %v — lancer go test ./internal/httpapi -run TestRoutesContract -update-routes", added)
		return
	}
	header := "# Inventaire figé des routes HTTP (TestRoutesContract, ADR 0039). Ajouter : -update-routes. Retirer : ADR.\n"
	if err := os.WriteFile(golden, []byte(header+strings.Join(got, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}
