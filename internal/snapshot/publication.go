package snapshot

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"bomstudio/internal/bom"
)

// Publication tracée (ADR 0033, vision §8 option A) : pas de comptes, des noms saisis ; les contrôles sont
// recalculés au moment de publier et copiés dans le snapshot avec les justifications.

var (
	// ErrPublicationBlocked : au moins un contrôle bloquant ; rien n'est publié.
	ErrPublicationBlocked = errors.New("publication blocked by a check")
	// ErrJustificationRequired : un avertissement n'a pas de justification écrite ; rien n'est publié.
	ErrJustificationRequired = errors.New("warning needs a written justification")
)

// PublishRequest : la note de décision, qui propose, qui valide, et une justification par code d'avertissement.
type PublishRequest struct {
	Note           string
	ProposedBy     string
	ValidatedBy    string
	Justifications map[string]string
}

// Bornes de saisie (mêmes que la base, migration 00013).
const (
	maxNote = 4000
	maxName = 200
)

type trace struct {
	Note, ProposedBy, ValidatedBy *string
	Justifications                map[string]string
}

// normalize retire les espaces autour ; un champ vide devient absent (nil), jamais une chaîne vide stockée.
func (r PublishRequest) normalize() (trace, error) {
	opt := func(field, v string, maxLen int) (*string, error) {
		v = strings.TrimSpace(v)
		if v == "" {
			return nil, nil //nolint:nilnil // champ non saisi
		}
		if utf8.RuneCountInString(v) > maxLen {
			return nil, fmt.Errorf("%s: %d characters max: %w", field, maxLen, bom.ErrInvalidInput)
		}
		return &v, nil
	}
	var t trace
	var err error
	if t.Note, err = opt("note", r.Note, maxNote); err != nil {
		return t, err
	}
	if t.ProposedBy, err = opt("proposed_by", r.ProposedBy, maxName); err != nil {
		return t, err
	}
	if t.ValidatedBy, err = opt("validated_by", r.ValidatedBy, maxName); err != nil {
		return t, err
	}
	t.Justifications = map[string]string{}
	for code, text := range r.Justifications {
		j, err := opt("justification "+code, text, maxNote)
		if err != nil {
			return t, err
		}
		if j != nil {
			t.Justifications[code] = *j
		}
	}
	return t, nil
}

// gate refuse la publication si un contrôle bloque ou si un avertissement n'est pas justifié ; sinon il rend
// les contrôles à figer, avec leur justification (celles des contrôles qui ne sont pas en warn sont ignorées).
func gate(checks []bom.PublicationCheck, justifications map[string]string) ([]FrozenCheck, error) {
	var blocked, unjustified []string
	var causes []error
	out := make([]FrozenCheck, 0, len(checks))
	for _, c := range checks {
		fc := FrozenCheck{Code: c.Code, Level: string(c.Level), Title: c.Title, Detail: c.Detail}
		switch c.Level {
		case bom.CheckBlock:
			blocked = append(blocked, fmt.Sprintf("%s (%s)", c.Code, c.Detail))
			if c.Err != nil { // coût incomplet : l'erreur nommée de chaque pièce reste lisible (errors.Is)
				causes = append(causes, ErrIncompleteCost, c.Err)
			}
		case bom.CheckWarn:
			j, ok := justifications[c.Code]
			if !ok {
				unjustified = append(unjustified, c.Code)
				break
			}
			fc.Justification = &j
		}
		out = append(out, fc)
	}
	if len(blocked) > 0 {
		err := fmt.Errorf("%s: %w", strings.Join(blocked, ", "), ErrPublicationBlocked)
		if len(causes) > 0 {
			err = fmt.Errorf("%w: %w", err, errors.Join(causes...))
		}
		return nil, err
	}
	if len(unjustified) > 0 {
		slices.Sort(unjustified)
		return nil, fmt.Errorf("justify %s: %w", strings.Join(unjustified, ", "), ErrJustificationRequired)
	}
	return out, nil
}
