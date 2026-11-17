package main

import (
	"os"
	"strings"
	"testing"
)

func TestAssess(t *testing.T) {
	tests := []struct {
		name    string
		changes []Change
		want    int
		reason  string // fragment attendu dans la raison du fichier le plus risqué
	}{
		{"rien", nil, Cosmetic, ""},
		{"doc seule", []Change{{Path: "docs/v2/LANCER.md", Status: "M"}}, Cosmetic, "documentation"},
		{"style seul", []Change{{Path: "web/src/styles/tokens.css", Status: "M"}}, Cosmetic, "style"},
		{"fichier inconnu = routine", []Change{{Path: "web/src/views/Compare.tsx", Status: "M"}}, Routine, "non classé"},
		{"un domaine", []Change{{Path: "internal/supplier/service.go", Status: "M"}, {Path: "internal/supplier/service_test.go", Status: "M"}}, Routine, ""},
		{"deux domaines", []Change{{Path: "internal/supplier/service.go", Status: "M"}, {Path: "internal/catalog/service.go", Status: "M"}}, Sensitive, "plusieurs domaines"},
		{"migration", []Change{{Path: "migrations/00016_x.sql", Status: "A"}}, Sensitive, "migration"},
		{"contrat API", []Change{{Path: "internal/httpapi/boms.go", Status: "M"}}, Sensitive, "contrat"},
		{"dépendance", []Change{{Path: "go.mod", Status: "M"}}, Sensitive, "dépendance"},
		{"ADR", []Change{{Path: "docs/adr/0039-harnais.md", Status: "A"}}, Sensitive, "architecture"},
		{"spec", []Change{{Path: "docs/features.md", Status: "M"}}, Sensitive, "spécification"},
		{"money", []Change{{Path: "internal/money/money.go", Status: "M"}}, Critical, "montants"},
		{"snapshot", []Change{{Path: "internal/snapshot/service.go", Status: "M"}}, Critical, "immuabilité"},
		{"calcul du coût", []Change{{Path: "internal/bom/cost_method.go", Status: "M"}}, Critical, "coût"},
		{"CI", []Change{{Path: ".github/workflows/ci.yml", Status: "M"}}, Critical, "harnais"},
		{"AGENTS.md racine", []Change{{Path: "AGENTS.md", Status: "M"}}, Critical, "harnais"},
		{"le plus élevé l'emporte", []Change{{Path: "docs/x.md", Status: "M"}, {Path: "internal/money/exact.go", Status: "M"}}, Critical, "montants"},
		{"test supprimé", []Change{{Path: "internal/bom/tree_test.go", Status: "D"}}, Critical, "supprimé"},
		{"fonction de test retirée", []Change{{Path: "internal/supplier/s_test.go", Status: "M", Removed: []string{"func TestQuote(t *testing.T) {"}}}, Critical, "supprimée"},
		{"t.Skip ajouté", []Change{{Path: "internal/supplier/s_test.go", Status: "M", Added: []string{`	t.Skip("flaky")`}}}, Critical, "désactivé"},
		{"test.only ajouté", []Change{{Path: "web/e2e/compare.spec.ts", Status: "M", Added: []string{`test.only('x', async () => {})`}}}, Critical, "focalisé"},
		{"saut d'environnement toléré", []Change{{Path: "internal/supplier/s_test.go", Status: "M", Added: []string{`		t.Skip("DATABASE_URL absente")`}}}, Routine, ""},
		{"test ajouté reste routine", []Change{{Path: "internal/supplier/s_test.go", Status: "M", Added: []string{"func TestNew(t *testing.T) {"}}}, Routine, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Assess(tt.changes)
			if got.Level != tt.want {
				t.Fatalf("niveau = %d, attendu %d (%+v)", got.Level, tt.want, got.Findings)
			}
			if tt.reason != "" && !strings.Contains(got.Findings[0].Reason, tt.reason) {
				t.Errorf("raison = %q, attendu un texte contenant %q", got.Findings[0].Reason, tt.reason)
			}
		})
	}
}

func TestAssessIsDeterministic(t *testing.T) {
	in := []Change{{Path: "internal/catalog/a.go"}, {Path: "internal/bom/b.go"}, {Path: "docs/c.md"}}
	rev := []Change{in[2], in[1], in[0]}
	a, b := Assess(in), Assess(rev)
	if a.Level != b.Level || len(a.Findings) != len(b.Findings) {
		t.Fatalf("résultats différents selon l'ordre : %+v / %+v", a, b)
	}
	for i := range a.Findings {
		if a.Findings[i] != b.Findings[i] {
			t.Fatalf("constat %d différent : %+v / %+v", i, a.Findings[i], b.Findings[i])
		}
	}
}

func TestGlobRegexp(t *testing.T) {
	tests := []struct {
		glob, path string
		want       bool
	}{
		{"internal/money/**", "internal/money/exact.go", true},
		{"internal/money/**", "internal/moneyx/a.go", false},
		{"**/*.md", "README.md", true},
		{"**/*.md", "a/b/c.md", true},
		{"internal/httpapi/*.go", "internal/httpapi/boms.go", true},
		{"internal/httpapi/*.go", "internal/httpapi/sub/x.go", false},
		{"web/src/**/*.css", "web/src/tokens.css", true},
	}
	for _, tt := range tests {
		t.Run(tt.glob+" "+tt.path, func(t *testing.T) {
			if got := globRegexp(tt.glob).MatchString(tt.path); got != tt.want {
				t.Errorf("= %v, attendu %v", got, tt.want)
			}
		})
	}
}

// TestRulesMatchAgentsDoc empêche AGENTS.md d'annoncer des niveaux que l'outil n'applique plus
// (dérive de la doc des agents) : chaque niveau de la table doit y être nommé.
func TestRulesMatchAgentsDoc(t *testing.T) {
	data, err := os.ReadFile("../../AGENTS.md")
	if err != nil {
		t.Fatalf("lecture AGENTS.md : %v", err)
	}
	doc := string(data)
	for level := Cosmetic; level <= Critical; level++ {
		if !strings.Contains(doc, LevelName(level)) {
			t.Errorf("AGENTS.md ne nomme pas le niveau %q", LevelName(level))
		}
	}
	for _, must := range []string{"go run ./cmd/risk", "internal/money", "migrations/", ".github/"} {
		if !strings.Contains(doc, must) {
			t.Errorf("AGENTS.md ne mentionne pas %q, pourtant dans la table des niveaux", must)
		}
	}
}
