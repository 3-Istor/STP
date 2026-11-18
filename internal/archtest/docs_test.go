package archtest

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Dérive de la doc des agents (ADR 0040) : un guide qui ment à l'agent est pire que pas de guide.
// Ces tests échouent quand AGENTS.md, une skill ou un relecteur cite un fichier qui n'existe plus,
// oublie une skill, grossit au-delà de ce qu'un agent lit vraiment, ou donne l'écriture à un relecteur.

const (
	maxAgentsLines = 120
	maxSkillLines  = 100
)

func read(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root(t), rel)) //nolint:gosec // G304 : chemins fixes du dépôt, test uniquement
	if err != nil {
		t.Fatalf("lecture %s : %v", rel, err)
	}
	return string(data)
}

// frontmatter lit les clés simples `cle: valeur` entre les deux lignes `---` d'en-tête.
func frontmatter(t *testing.T, rel, text string) map[string]string {
	t.Helper()
	if !strings.HasPrefix(text, "---\n") {
		t.Fatalf("%s : en-tête YAML absent", rel)
	}
	end := strings.Index(text[4:], "\n---")
	if end < 0 {
		t.Fatalf("%s : en-tête YAML non fermé", rel)
	}
	fm := map[string]string{}
	for _, l := range strings.Split(text[4:4+end], "\n") {
		if k, v, ok := strings.Cut(l, ":"); ok {
			fm[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return fm
}

func dirNames(t *testing.T, rel, suffix string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root(t), rel))
	if err != nil {
		t.Fatalf("lecture %s : %v", rel, err)
	}
	var names []string
	for _, e := range entries {
		if suffix == "" && e.IsDir() {
			names = append(names, e.Name())
		} else if suffix != "" && strings.HasSuffix(e.Name(), suffix) {
			names = append(names, strings.TrimSuffix(e.Name(), suffix))
		}
	}
	sort.Strings(names)
	return names
}

func TestAgentsMapStaysShort(t *testing.T) {
	if n := strings.Count(read(t, "AGENTS.md"), "\n"); n > maxAgentsLines {
		t.Errorf("AGENTS.md fait %d lignes (> %d) : c'est une carte, déplacer le détail dans docs/agents/", n, maxAgentsLines)
	}
}

func TestSkillsAreShortAndListed(t *testing.T) {
	agents := read(t, "AGENTS.md")
	skills := dirNames(t, ".claude/skills", "")
	if len(skills) == 0 {
		t.Fatal("aucune skill dans .claude/skills")
	}
	for _, name := range skills {
		rel := ".claude/skills/" + name + "/SKILL.md"
		text := read(t, rel)
		fm := frontmatter(t, rel, text)
		if fm["name"] != name {
			t.Errorf("%s : name = %q, attendu le nom du dossier %q", rel, fm["name"], name)
		}
		if d := fm["description"]; len(d) < 40 || len(d) > 1024 {
			t.Errorf("%s : description de %d caractères (40 à 1024 : quoi + quand l'utiliser)", rel, len(d))
		}
		if n := strings.Count(text, "\n"); n > maxSkillLines {
			t.Errorf("%s : %d lignes (> %d) : découper ou renvoyer vers un fichier de référence", rel, n, maxSkillLines)
		}
		if !strings.Contains(agents, "`"+name+"`") {
			t.Errorf("skill %q absente de la table des skills d'AGENTS.md", name)
		}
	}
}

var writeTools = regexp.MustCompile(`\b(Edit|Write|MultiEdit|NotebookEdit)\b`)

func TestReviewersAreReadOnlyAndListed(t *testing.T) {
	agents := read(t, "AGENTS.md")
	reviewers := dirNames(t, ".claude/agents", ".md")
	if len(reviewers) < 4 {
		t.Fatalf("%d relecteurs, 4 attendus (code, architecture, sécurité, métier)", len(reviewers))
	}
	for _, name := range reviewers {
		rel := ".claude/agents/" + name + ".md"
		fm := frontmatter(t, rel, read(t, rel))
		if fm["name"] != name {
			t.Errorf("%s : name = %q, attendu %q", rel, fm["name"], name)
		}
		if fm["tools"] == "" || writeTools.MatchString(fm["tools"]) {
			t.Errorf("%s : tools = %q ; un relecteur est en lecture seule (liste explicite, sans Edit/Write)", rel, fm["tools"])
		}
		if !strings.Contains(agents, "`"+name+"`") {
			t.Errorf("relecteur %q absent d'AGENTS.md", name)
		}
	}
}

// backtickPath repère les chemins du dépôt cités entre accents graves (sans joker ni gabarit).
var backtickPath = regexp.MustCompile("`((?:\\.claude|\\.github|\\.githooks|cmd|internal|docs|migrations|scripts|web)/[^`*<>{}\\s|]+|[A-Z]+\\.md|Makefile)`")

func TestCitedPathsExist(t *testing.T) {
	files := []string{"AGENTS.md", "REVIEW.md", "docs/harnais.md"}
	for _, s := range dirNames(t, ".claude/skills", "") {
		files = append(files, ".claude/skills/"+s+"/SKILL.md")
	}
	for _, r := range dirNames(t, ".claude/agents", ".md") {
		files = append(files, ".claude/agents/"+r+".md")
	}
	for _, f := range files {
		for _, m := range backtickPath.FindAllStringSubmatch(read(t, f), -1) {
			p := strings.TrimRight(m[1], ".,:;)")
			if strings.HasPrefix(p, "docs/adr/NNNN") {
				continue // gabarit
			}
			if _, err := os.Stat(filepath.Join(root(t), p)); err != nil {
				t.Errorf("%s cite `%s`, introuvable dans le dépôt", f, p)
			}
		}
	}
}
