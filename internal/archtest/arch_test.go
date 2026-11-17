// Package archtest vérifie les frontières d'architecture d'AGENTS.md §2 et §3 sur le code réel.
// Capteur calculé (ADR 0039) : il échoue dès qu'un import ou un type interdit apparaît, sans IA.
package archtest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const module = "bomstudio/"

// domains : le domaine ne connaît ni écran, ni réseau, ni base, ni fournisseur de prix (AGENTS §2).
var domains = []string{"bom", "catalog", "money", "pricing", "pricesim", "sampledata", "snapshot", "supplier"}

type goFile struct {
	path    string // relatif à la racine du dépôt
	pkgDir  string // dossier du paquet, relatif
	imports []string
	ast     *ast.File
	fset    *token.FileSet
}

func root(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
		t.Fatalf("racine du dépôt introuvable depuis %s : %v", dir, err)
	}
	return dir
}

// sources lit tous les fichiers Go hors tests, hors code généré (sqlc), sous internal/ et cmd/.
func sources(t *testing.T) []goFile {
	t.Helper()
	base := root(t)
	var files []goFile
	for _, top := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(base, top), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return nil
			}
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, p, nil, parser.ParseComments)
			if err != nil {
				return err
			}
			if ast.IsGenerated(f) {
				return nil
			}
			rel, _ := filepath.Rel(base, p)
			rel = filepath.ToSlash(rel)
			gf := goFile{path: rel, pkgDir: filepath.ToSlash(filepath.Dir(rel)), ast: f, fset: fset}
			for _, imp := range f.Imports {
				v, _ := strconv.Unquote(imp.Path.Value)
				gf.imports = append(gf.imports, v)
			}
			files = append(files, gf)
			return nil
		})
		if err != nil {
			t.Fatalf("parcours de %s : %v", top, err)
		}
	}
	if len(files) < 50 {
		t.Fatalf("seulement %d fichiers lus : le test ne regarde pas le bon dossier", len(files))
	}
	return files
}

// domainOf renvoie le domaine du paquet racine (internal/<d>), "" pour ses sous-paquets d'adapters.
func domainOf(pkgDir string) string {
	parts := strings.Split(pkgDir, "/")
	if len(parts) != 2 || parts[0] != "internal" {
		return ""
	}
	for _, d := range domains {
		if parts[1] == d {
			return d
		}
	}
	return ""
}

func isAdapter(imp string) bool {
	return strings.HasPrefix(imp, module+"internal/") &&
		(strings.HasSuffix(imp, "store") || strings.HasSuffix(imp, "db") || strings.HasSuffix(imp, "client"))
}

func TestBoundaries(t *testing.T) {
	type violation struct{ file, imp, rule string }
	var got []violation
	for _, f := range sources(t) {
		dom := domainOf(f.pkgDir)
		for _, imp := range f.imports {
			switch {
			case dom != "" && (imp == "net/http" || strings.HasPrefix(imp, "net/http/")):
				got = append(got, violation{f.path, imp, "le domaine ne connaît pas le réseau (AGENTS §2)"})
			case dom != "" && (imp == "database/sql" || strings.HasPrefix(imp, "github.com/jackc/")):
				got = append(got, violation{f.path, imp, "le domaine ne connaît pas la base : passer par un *store"})
			case dom != "" && (isAdapter(imp) || imp == module+"internal/httpapi"):
				got = append(got, violation{f.path, imp, "le domaine ne dépend pas d'un adapter"})
			case dom == "money" && strings.HasPrefix(imp, module):
				got = append(got, violation{f.path, imp, "money ne dépend d'aucun domaine (AGENTS §2)"})
			case dom != "" && imp == "math/rand" || dom != "" && imp == "math/rand/v2":
				got = append(got, violation{f.path, imp, "aléa interdit dans le domaine : résultats reproductibles (AGENTS §3)"})
			case f.pkgDir == "internal/httpapi" && (isAdapter(imp) || strings.HasPrefix(imp, "github.com/jackc/")):
				got = append(got, violation{f.path, imp, "handler fin : appeler un service, jamais la base (AGENTS §4)"})
			case imp == module+"internal/httpapi" && !strings.HasPrefix(f.pkgDir, "cmd/"):
				got = append(got, violation{f.path, imp, "seul cmd/ câble le routeur"})
			case strings.HasPrefix(imp, "github.com/anthropics/") || strings.HasPrefix(imp, "github.com/openai/"):
				got = append(got, violation{f.path, imp, "SDK d'IA hors de l'implémentation d'une interface du domaine (AGENTS §3)"})
			}
		}
	}
	for _, v := range got {
		t.Errorf("%s importe %q : %s", v.file, v.imp, v.rule)
	}
}

// TestNoFloatForAmounts : jamais de float pour un montant, un taux ou un prix (AGENTS §3).
// Le code de production n'en a aucun besoin aujourd'hui : toute apparition est signalée.
func TestNoFloatForAmounts(t *testing.T) {
	var found []string
	for _, f := range sources(t) {
		ast.Inspect(f.ast, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.Ident:
				if x.Name == "float64" || x.Name == "float32" {
					found = append(found, f.fset.Position(x.Pos()).String())
				}
			case *ast.SelectorExpr:
				if id, ok := x.X.(*ast.Ident); ok && id.Name == "strconv" && x.Sel.Name == "ParseFloat" {
					found = append(found, f.fset.Position(x.Pos()).String())
				}
			}
			return true
		})
	}
	sort.Strings(found)
	for _, pos := range found {
		t.Errorf("%s : float interdit pour un montant ; utiliser decimal.Decimal ou money.Money", relPos(t, pos))
	}
}

// TestDomainHasNoClock : le domaine reçoit l'instant en paramètre, il ne lit pas l'horloge (AGENTS §3).
func TestDomainHasNoClock(t *testing.T) {
	for _, f := range sources(t) {
		if domainOf(f.pkgDir) == "" {
			continue
		}
		ast.Inspect(f.ast, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == "time" && (sel.Sel.Name == "Now" || sel.Sel.Name == "Since") {
				t.Errorf("%s : time.%s dans le domaine ; injecter l'instant (reproductibilité)", relPos(t, f.fset.Position(call.Pos()).String()), sel.Sel.Name)
			}
			return true
		})
	}
}

func relPos(t *testing.T, pos string) string {
	if rel, err := filepath.Rel(root(t), pos); err == nil {
		return filepath.ToSlash(rel)
	}
	return pos
}
