package fsblob_test

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"bomstudio/internal/viz3d"
	"bomstudio/internal/viz3d/fsblob"
)

func TestStore(t *testing.T) {
	dir := t.TempDir()
	s, err := fsblob.New(filepath.Join(dir, "models"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	data := []byte("glTF-bytes")

	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"absent avant Put", func(t *testing.T) {
			ok, err := s.Exists(ctx, "models/ab/abc.glb")
			if err != nil || ok {
				t.Fatalf("exists = %v, %v", ok, err)
			}
			if _, _, err := s.Get(ctx, "models/ab/abc.glb"); !errors.Is(err, viz3d.ErrBlobNotFound) {
				t.Fatalf("get : err = %v", err)
			}
		}},
		{"Put crée les dossiers, Get relit la taille et le contenu", func(t *testing.T) {
			if err := s.Put(ctx, "models/ab/abc.glb", bytes.NewReader(data)); err != nil {
				t.Fatal(err)
			}
			rc, size, err := s.Get(ctx, "models/ab/abc.glb")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = rc.Close() }()
			got, _ := io.ReadAll(rc)
			if size != int64(len(data)) || !bytes.Equal(got, data) {
				t.Fatalf("relu %d octets : %q", size, got)
			}
			if ok, _ := s.Exists(ctx, "models/ab/abc.glb"); !ok {
				t.Fatal("exists = false après Put")
			}
			if _, err := os.Stat(filepath.Join(dir, "models", "models", "ab", "abc.glb.tmp")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("le fichier temporaire est resté")
			}
		}},
		{"Put remplace", func(t *testing.T) {
			if err := s.Put(ctx, "models/ab/abc.glb", bytes.NewReader([]byte("v2"))); err != nil {
				t.Fatal(err)
			}
			rc, size, err := s.Get(ctx, "models/ab/abc.glb")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = rc.Close() }()
			if size != 2 {
				t.Fatalf("taille %d", size)
			}
		}},
		{"clé hors du répertoire refusée (traversée)", func(t *testing.T) {
			for _, key := range []string{"../outside.glb", "/etc/passwd", "models/../../x", "Models/X.glb"} {
				if err := s.Put(ctx, key, bytes.NewReader(data)); !errors.Is(err, viz3d.ErrInvalidModel) {
					t.Fatalf("put %q : err = %v", key, err)
				}
				if _, _, err := s.Get(ctx, key); !errors.Is(err, viz3d.ErrInvalidModel) {
					t.Fatalf("get %q : err = %v", key, err)
				}
				if _, err := s.Exists(ctx, key); !errors.Is(err, viz3d.ErrInvalidModel) {
					t.Fatalf("exists %q : err = %v", key, err)
				}
			}
			if _, err := os.Stat(filepath.Join(dir, "outside.glb")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("un fichier a été écrit hors du répertoire")
			}
		}},
		{"lien symbolique vers l'extérieur : non suivi", func(t *testing.T) {
			outside := filepath.Join(dir, "secret.txt")
			if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, filepath.Join(dir, "models", "link.glb")); err != nil {
				t.Skipf("symlink non supporté : %v", err)
			}
			if _, _, err := s.Get(ctx, "link.glb"); err == nil {
				t.Fatal("le lien vers l'extérieur a été suivi")
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}
