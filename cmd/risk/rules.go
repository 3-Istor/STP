package main

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
)

// Niveaux de risque d'un changement (voir AGENTS.md « Niveaux de risque » et ADR 0039).
// Le niveau dépend du changement, jamais de l'auteur ni de la confiance de l'agent.
const (
	Cosmetic  = 0
	Routine   = 1
	Sensitive = 2
	Critical  = 3
)

// LevelName donne le nom affiché d'un niveau.
func LevelName(level int) string {
	switch level {
	case Cosmetic:
		return "0 Cosmétique"
	case Routine:
		return "1 Routine"
	case Sensitive:
		return "2 Sensible"
	case Critical:
		return "3 Critique"
	}
	return fmt.Sprintf("%d inconnu", level)
}

// rule associe des chemins (globs, `**` = n'importe quel sous-chemin) à un niveau.
type rule struct {
	level  int
	reason string
	globs  []string
}

// rules est la seule table des niveaux : AGENTS.md et CODEOWNERS la recopient, le test
// TestRulesMatchAgentsDoc vérifie qu'ils ne divergent pas. Un fichier qui ne correspond à
// aucune règle est au niveau 1 (inconnu = routine, jamais cosmétique).
var rules = []rule{
	{Critical, "règle métier non négociable : montants (AGENTS §3)", []string{"internal/money/**"}},
	{Critical, "immuabilité des snapshots (AGENTS §3, ADR 0038)", []string{"internal/snapshot/**", "migrations/*immutab*", "migrations/*snapshot*"}},
	{Critical, "calcul du coût et de ses causes", []string{"internal/bom/cost*.go", "internal/bom/totals*.go", "internal/bom/bridge*.go", "internal/bom/rootbudget*.go"}},
	{Critical, "harnais : règles, hooks, CI, relecteurs", []string{
		".github/**", ".githooks/**", ".claude/**", "AGENTS.md", "CLAUDE.md", "REVIEW.md", "SECURITY.md",
		"cmd/risk/**", "internal/archtest/**", "scripts/**", ".golangci.yml", "docs/agents/**",
	}},
	{Sensitive, "migration de schéma (jamais modifier une migration commitée)", []string{"migrations/**"}},
	{Sensitive, "contrat /api/v1 (routes, JSON)", []string{"internal/httpapi/*.go", "internal/httpapi/testdata/**"}},
	{Sensitive, "dépendance ou image d'exécution", []string{"go.mod", "go.sum", "web/package.json", "web/package-lock.json", "Dockerfile", "compose.yaml", "web/Dockerfile", "web/nginx*"}},
	{Sensitive, "jeu de données de démonstration", []string{"internal/sampledata/**"}},
	{Sensitive, "décision d'architecture", []string{"docs/adr/**"}},
	{Sensitive, "spécification fonctionnelle (AGENTS §11)", []string{"docs/features.md"}},
	{Cosmetic, "documentation", []string{"docs/**", "*.md", "**/*.md"}},
	{Cosmetic, "style ou capture", []string{"web/src/**/*.css", "**/*.png", "**/*.jpg", "**/*.svg"}},
}

var globCache = map[string]*regexp.Regexp{}

// globRegexp traduit un glob en expression régulière ancrée :
// `**/` = zéro ou plusieurs dossiers, `**` = tout, `*` = tout sauf `/`.
func globRegexp(glob string) *regexp.Regexp {
	if re, ok := globCache[glob]; ok {
		return re
	}
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(glob); i++ {
		switch {
		case strings.HasPrefix(glob[i:], "**/"):
			b.WriteString("(?:.*/)?")
			i += 2
		case strings.HasPrefix(glob[i:], "**"):
			b.WriteString(".*")
			i++
		case glob[i] == '*':
			b.WriteString("[^/]*")
		default:
			b.WriteString(regexp.QuoteMeta(string(glob[i])))
		}
	}
	b.WriteString("$")
	re := regexp.MustCompile(b.String())
	globCache[glob] = re
	return re
}

// Change est un fichier modifié, avec les lignes ajoutées et supprimées utiles aux règles de tests.
type Change struct {
	Path    string
	Status  string // A, M, D, R…
	Added   []string
	Removed []string
}

// Finding explique pourquoi un fichier porte un niveau.
type Finding struct {
	Path   string `json:"path"`
	Level  int    `json:"level"`
	Reason string `json:"reason"`
}

// Assessment est le résultat : niveau global (le plus élevé) et raisons.
type Assessment struct {
	Level    int       `json:"level"`
	Findings []Finding `json:"findings"`
	Domains  []string  `json:"domains"`
}

// classifyPath donne le niveau d'un chemin selon la première règle la plus élevée qui correspond.
func classifyPath(p string) Finding {
	best := Finding{Path: p, Level: Routine, Reason: "code ou fichier non classé (inconnu = routine)"}
	matched := false
	for _, r := range rules {
		for _, g := range r.globs {
			if globRegexp(g).MatchString(p) {
				if !matched || r.level > best.Level {
					best = Finding{Path: p, Level: r.level, Reason: r.reason}
					matched = true
				}
				break
			}
		}
	}
	return best
}

var (
	skipAdded = regexp.MustCompile(`\bt\.Skip(f|Now)?\(|\btest\.(skip|fixme)\(|\b(it|test|describe)\.only\(`)
	// Un saut lié à l'environnement (pas de base, pas de WebGL) est la convention du dépôt :
	// la CI fournit cet environnement, le test y tourne. Tout autre saut est un affaiblissement.
	envSkip     = regexp.MustCompile(`DATABASE_URL|injoignable|indisponible|absente?\b`)
	testRemoved = regexp.MustCompile(`^\s*func (Test|Fuzz)\w*\(|^\s*(test|it)\(`)
)

// testWeakened détecte un test supprimé, désactivé ou focalisé (AGENTS §11) : niveau 3.
func testWeakened(c Change) (string, bool) {
	isTest := strings.HasSuffix(c.Path, "_test.go") || strings.HasSuffix(c.Path, ".spec.ts") || strings.HasSuffix(c.Path, ".test.ts")
	if !isTest {
		return "", false
	}
	if c.Status == "D" {
		return "fichier de test supprimé", true
	}
	for _, l := range c.Added {
		if skipAdded.MatchString(l) && !envSkip.MatchString(l) {
			return "test désactivé ou focalisé ajouté : " + strings.TrimSpace(l), true
		}
	}
	for _, l := range c.Removed {
		if testRemoved.MatchString(l) {
			return "fonction de test supprimée : " + strings.TrimSpace(l), true
		}
	}
	return "", false
}

// domainOf renvoie le domaine Go (internal/<d>) d'un fichier non test, ou "".
func domainOf(p string) string {
	if !strings.HasPrefix(p, "internal/") || strings.HasSuffix(p, "_test.go") || path.Ext(p) != ".go" {
		return ""
	}
	parts := strings.Split(p, "/")
	if len(parts) < 3 || parts[1] == "httpapi" || parts[1] == "archtest" {
		return ""
	}
	return parts[1]
}

// Assess calcule le niveau d'un ensemble de changements. Déterministe : même entrée, même sortie.
func Assess(changes []Change) Assessment {
	a := Assessment{Level: Cosmetic}
	domains := map[string]bool{}
	for _, c := range changes {
		f := classifyPath(c.Path)
		if reason, weak := testWeakened(c); weak {
			f = Finding{Path: c.Path, Level: Critical, Reason: reason}
		}
		a.Findings = append(a.Findings, f)
		a.Level = max(a.Level, f.Level)
		if d := domainOf(c.Path); d != "" {
			domains[d] = true
		}
	}
	for d := range domains {
		a.Domains = append(a.Domains, d)
	}
	sort.Strings(a.Domains)
	if len(a.Domains) >= 2 && a.Level < Sensitive {
		a.Level = Sensitive
		a.Findings = append(a.Findings, Finding{
			Path: strings.Join(a.Domains, ", "), Level: Sensitive,
			Reason: "plusieurs domaines touchés : frontière à vérifier",
		})
	}
	sort.SliceStable(a.Findings, func(i, j int) bool {
		if a.Findings[i].Level != a.Findings[j].Level {
			return a.Findings[i].Level > a.Findings[j].Level
		}
		return a.Findings[i].Path < a.Findings[j].Path
	})
	return a
}

// Checkpoint décrit ce que le niveau exige (contrat partagé avec AGENTS.md et REVIEW.md).
type Checkpoint struct {
	Human     string
	Reviewers []string
}

// CheckpointFor renvoie le point de contrôle humain et les relecteurs IA d'un niveau.
func CheckpointFor(level int) Checkpoint {
	switch level {
	case Cosmetic:
		return Checkpoint{Human: "aucun en direct : la moulinette suffit"}
	case Routine:
		return Checkpoint{Human: "1 humain lit le résumé de preuves (verify-summary.md) avant merge", Reviewers: []string{"relecteur-code"}}
	case Sensitive:
		return Checkpoint{Human: "1 humain valide le plan avant le code, puis approuve le diff", Reviewers: []string{"relecteur-code", "relecteur-architecture"}}
	default:
		return Checkpoint{Human: "2 humains dont un CODEOWNER : plan validé avant le code, diff sensible relu ligne à ligne", Reviewers: []string{"relecteur-code", "relecteur-architecture", "relecteur-securite", "relecteur-metier"}}
	}
}
