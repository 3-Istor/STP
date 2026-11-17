package snapshot_test

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"bomstudio/internal/bom"
	"bomstudio/internal/pricing"
	"bomstudio/internal/snapshot"
)

// Publication tracée (ADR 0033). Le brouillon de référence est en prix en direct et sans source de contrats :
// prices_frozen et expired_contracts sont en warn, tout le reste ok.
func TestPublishChecksAndTrace(t *testing.T) {
	both := map[string]string{bom.CheckPricesFrozen: "figé à la publication", bom.CheckExpiredContracts: "contrats revus à la main"}
	tests := []struct {
		name    string
		prepare func(t *testing.T, f fixture, b bom.BOM)
		req     snapshot.PublishRequest
		wantErr []error
	}{
		{name: "sans justification : 422, rien n'est publié", wantErr: []error{snapshot.ErrJustificationRequired}},
		{
			name:    "une justification sur deux : 422",
			req:     snapshot.PublishRequest{Justifications: map[string]string{bom.CheckPricesFrozen: "ok"}},
			wantErr: []error{snapshot.ErrJustificationRequired},
		},
		{
			name:    "justification blanche : comme absente",
			req:     snapshot.PublishRequest{Justifications: map[string]string{bom.CheckPricesFrozen: "  ", bom.CheckExpiredContracts: "x"}},
			wantErr: []error{snapshot.ErrJustificationRequired},
		},
		{
			name: "pièce sans cotation : bloqué (409), même justifié, et l'erreur nommée reste lisible",
			prepare: func(t *testing.T, f fixture, b bom.BOM) {
				_, nodes, err := f.boms.Tree(t.Context(), b.ID)
				if err != nil {
					t.Fatal(err)
				}
				f.addPart(t, b.ID, nodes[0].ID, f.volant, f.volantO, "1")
			},
			req:     snapshot.PublishRequest{Justifications: both},
			wantErr: []error{snapshot.ErrPublicationBlocked, pricing.ErrNoQuote},
		},
		{
			name:    "nom trop long : refusé avant tout",
			req:     snapshot.PublishRequest{ProposedBy: string(make([]byte, 201)), Justifications: both},
			wantErr: []error{bom.ErrInvalidInput},
		},
		{
			name: "avertissements justifiés : publié avec la trace",
			req: snapshot.PublishRequest{Note: "  Jalon de lancement ", ProposedBy: "Arthur P.", ValidatedBy: "Joe",
				Justifications: map[string]string{bom.CheckPricesFrozen: "figé à la publication",
					bom.CheckExpiredContracts: "contrats revus à la main", bom.CheckBudgets: "ignorée : pas en warn"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			b, _ := f.newDraft(t)
			if tt.prepare != nil {
				tt.prepare(t, f, b)
			}
			s, err := f.snaps.Publish(t.Context(), b.ID, t1, tt.req)
			for _, want := range tt.wantErr {
				if !errors.Is(err, want) {
					t.Errorf("err = %v, want %v", err, want)
				}
			}
			if len(tt.wantErr) > 0 {
				if n := f.snapshotCount(t, b.ID); n != 0 {
					t.Fatalf("%d snapshot(s) publiés malgré le refus", n)
				}
				return
			}
			if err != nil {
				t.Fatalf("publish: %v", err)
			}
			p := f.get(t, s.ID)
			if p.Snapshot.Note == nil || *p.Snapshot.Note != "Jalon de lancement" || *p.Snapshot.ProposedBy != "Arthur P." ||
				*p.Snapshot.ValidatedBy != "Joe" {
				t.Errorf("trace = %v %v %v", p.Snapshot.Note, p.Snapshot.ProposedBy, p.Snapshot.ValidatedBy)
			}
			if len(p.Checks) != 7 {
				t.Fatalf("%d checks frozen, want 7", len(p.Checks))
			}
			for _, c := range p.Checks {
				switch c.Code {
				case bom.CheckPricesFrozen, bom.CheckExpiredContracts:
					if c.Level != "warn" || c.Justification == nil {
						t.Errorf("%s = %s, justification %v, want warn justified", c.Code, c.Level, c.Justification)
					}
				default:
					if c.Level != "ok" || c.Justification != nil {
						t.Errorf("%s = %s, justification %v, want ok without justification", c.Code, c.Level, c.Justification)
					}
				}
			}
		})
	}
}

// La trace et les contrôles figés sont immuables, comme le reste du snapshot (ADR 0006, 0033).
func TestPublishedTraceIsImmutable(t *testing.T) {
	f := newFixture(t)
	b, _ := f.newDraft(t)
	s, err := f.snaps.Publish(t.Context(), b.ID, t1, snapshot.PublishRequest{ValidatedBy: "Joe", Justifications: map[string]string{
		bom.CheckPricesFrozen: "x", bom.CheckExpiredContracts: "y"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		`UPDATE bom_snapshots SET validated_by = 'Quelqu''un d''autre' WHERE id = $1`,
		`UPDATE bom_snapshot_checks SET justification = 'réécrite' WHERE snapshot_id = $1 AND justification IS NOT NULL`,
		`DELETE FROM bom_snapshot_checks WHERE snapshot_id = $1`,
	} {
		t.Run(sql, func(t *testing.T) {
			sp, err := f.tx.Begin(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = sp.Rollback(t.Context()) }()
			_, err = sp.Exec(t.Context(), sql, s.ID)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != restrictViolation {
				t.Errorf("err = %v, want SQLSTATE %s", err, restrictViolation)
			}
		})
	}
}
