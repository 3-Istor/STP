// Commande risk : calcule le niveau de risque (0–3) d'un changement à partir du diff git.
//
//	go run ./cmd/risk                 # copie de travail comparée à origin/main
//	go run ./cmd/risk -base HEAD~1    # dernier commit
//	go run ./cmd/risk -staged         # index (hook pre-commit)
//	go run ./cmd/risk -json           # sortie machine (CI)
//
// Le niveau dépend du changement, pas de l'auteur (voir ADR 0039, AGENTS.md).
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"
)

func main() {
	base := flag.String("base", "", "référence de comparaison (défaut : merge-base avec origin/main, sinon main)")
	staged := flag.Bool("staged", false, "analyser l'index git (pre-commit)")
	asJSON := flag.Bool("json", false, "sortie JSON")
	flag.Parse()

	ref := *base
	if ref == "" && !*staged {
		ref = defaultBase()
	}
	changes, err := collect(ref, *staged)
	if err != nil {
		slog.Error("risk: lecture du diff", "base", ref, "err", err)
		os.Exit(2)
	}
	a := Assess(changes)
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(a); err != nil {
			slog.Error("risk: encodage", "err", err)
			os.Exit(2)
		}
		return
	}
	writeMarkdown(os.Stdout, a, ref)
}

func defaultBase() string {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	for _, cand := range []string{"origin/main", "main"} {
		out, err := exec.CommandContext(ctx, "git", "merge-base", "HEAD", cand).Output() //nolint:gosec // G204 : cand vient d'une liste fixe
		if err == nil {
			return strings.TrimSpace(string(out))
		}
	}
	return "HEAD"
}

// gitTimeout borne chaque appel à git : un dépôt bloqué ne fige pas le hook pre-commit.
const gitTimeout = 30 * time.Second

func git(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec // G204 : binaire fixe (git), arguments construits par ce programme, pas de shell
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// collect lit les fichiers modifiés et, pour les tests, les lignes ajoutées et supprimées.
func collect(ref string, staged bool) ([]Change, error) {
	args := []string{"diff", "--no-renames", "--name-status"}
	if staged {
		args = append(args, "--cached")
	} else {
		args = append(args, ref)
	}
	out, err := git(args...)
	if err != nil {
		return nil, err
	}
	var changes []Change
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		fields := strings.Split(sc.Text(), "\t")
		if len(fields) < 2 {
			continue
		}
		changes = append(changes, Change{Status: fields[0][:1], Path: fields[len(fields)-1]})
	}
	if !staged {
		changes = append(changes, untracked()...)
	}
	for i, c := range changes {
		if !strings.HasSuffix(c.Path, "_test.go") && !strings.HasSuffix(c.Path, ".spec.ts") && !strings.HasSuffix(c.Path, ".test.ts") {
			continue
		}
		diffArgs := []string{"diff", "--no-renames", "-U0"}
		if staged {
			diffArgs = append(diffArgs, "--cached")
		} else {
			diffArgs = append(diffArgs, ref)
		}
		diff, err := git(append(diffArgs, "--", c.Path)...)
		if err != nil {
			return nil, err
		}
		changes[i].Added, changes[i].Removed = splitDiff(diff)
	}
	return changes, nil
}

// untracked ajoute les fichiers nouveaux pas encore indexés (un agent qui n'a pas fait `git add`).
func untracked() []Change {
	out, err := git("ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil
	}
	var cs []Change
	for _, p := range strings.Fields(string(out)) {
		c := Change{Path: p, Status: "A"}
		if data, err := os.ReadFile(p); err == nil { //nolint:gosec // chemin issu de git ls-files
			c.Added = strings.Split(string(data), "\n")
		}
		cs = append(cs, c)
	}
	return cs
}

func splitDiff(diff []byte) (added, removed []string) {
	for _, l := range strings.Split(string(diff), "\n") {
		switch {
		case strings.HasPrefix(l, "+++"), strings.HasPrefix(l, "---"):
		case strings.HasPrefix(l, "+"):
			added = append(added, l[1:])
		case strings.HasPrefix(l, "-"):
			removed = append(removed, l[1:])
		}
	}
	return added, removed
}

func writeMarkdown(w io.Writer, a Assessment, ref string) {
	var b strings.Builder
	cp := CheckpointFor(a.Level)
	fmt.Fprintf(&b, "## Niveau de risque : %s\n\n", LevelName(a.Level))
	if ref != "" {
		fmt.Fprintf(&b, "Comparé à `%s`. ", ref)
	}
	fmt.Fprintf(&b, "Point de contrôle humain : **%s**.\n\n", cp.Human)
	if len(cp.Reviewers) > 0 {
		fmt.Fprintf(&b, "Relecteurs IA en contexte neuf : %s.\n\n", strings.Join(cp.Reviewers, ", "))
	}
	if len(a.Findings) == 0 {
		b.WriteString("Aucun fichier modifié.\n")
	} else {
		b.WriteString("| Niveau | Fichier | Raison |\n|---|---|---|\n")
		shown := 0
		for _, f := range a.Findings {
			if f.Level == Cosmetic && a.Level > Cosmetic {
				continue // le cosmétique n'apprend rien quand un fichier plus risqué est là
			}
			if shown == 40 {
				fmt.Fprintf(&b, "| … | %d autres fichiers | |\n", len(a.Findings)-shown)
				break
			}
			fmt.Fprintf(&b, "| %d | `%s` | %s |\n", f.Level, f.Path, f.Reason)
			shown++
		}
	}
	if _, err := io.WriteString(w, b.String()); err != nil {
		slog.Error("risk: écriture", "err", err)
		os.Exit(2)
	}
}
