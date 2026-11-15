// Package fsblob implémente viz3d.BlobStore sur le système de fichiers, sous un répertoire racine
// (MODELS_DIR). Chaque clé est un chemin relatif validé par viz3d.ValidBlobKey et ouvert via os.Root :
// aucune clé ne peut sortir du répertoire, même par lien symbolique. Un adapter S3 viendrait à côté,
// derrière la même interface (ADR 0031, option 2b).
package fsblob

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"

	"bomstudio/internal/viz3d"
)

// Store range les fichiers sous dir.
type Store struct {
	dir string
}

var _ viz3d.BlobStore = (*Store)(nil)

// New crée le répertoire s'il manque et renvoie le store.
func New(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("models dir %q: %w", dir, err)
	}
	return &Store{dir: dir}, nil
}

// Dir est le répertoire racine.
func (s *Store) Dir() string { return s.dir }

func (s *Store) open() (*os.Root, error) {
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return nil, fmt.Errorf("open models dir %q: %w", s.dir, err)
	}
	return root, nil
}

func checkKey(key string) error {
	if !viz3d.ValidBlobKey(key) {
		return fmt.Errorf("blob key %q: %w", key, viz3d.ErrInvalidModel)
	}
	return nil
}

// Put écrit le fichier dans un temporaire du même dossier puis le renomme : un lecteur ne voit jamais un
// fichier à moitié écrit.
func (s *Store) Put(_ context.Context, key string, r io.Reader) error {
	if err := checkKey(key); err != nil {
		return err
	}
	root, err := s.open()
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	if dir := path.Dir(key); dir != "." {
		if err := root.MkdirAll(dir, 0o750); err != nil {
			return fmt.Errorf("put %q: %w", key, err)
		}
	}
	tmp := key + ".tmp"
	f, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o640)
	if err != nil {
		return fmt.Errorf("put %q: %w", key, err)
	}
	if _, err := io.Copy(f, r); err != nil {
		_ = f.Close()
		_ = root.Remove(tmp)
		return fmt.Errorf("put %q: write: %w", key, err)
	}
	if err := f.Close(); err != nil {
		_ = root.Remove(tmp)
		return fmt.Errorf("put %q: close: %w", key, err)
	}
	if err := root.Rename(tmp, key); err != nil {
		_ = root.Remove(tmp)
		return fmt.Errorf("put %q: rename: %w", key, err)
	}
	return nil
}

// Get ouvre le fichier et donne sa taille ; viz3d.ErrBlobNotFound s'il n'existe pas.
func (s *Store) Get(_ context.Context, key string) (io.ReadCloser, int64, error) {
	if err := checkKey(key); err != nil {
		return nil, 0, err
	}
	root, err := s.open()
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = root.Close() }()
	f, err := root.Open(key)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, 0, fmt.Errorf("get %q: %w", key, viz3d.ErrBlobNotFound)
	}
	if err != nil {
		return nil, 0, fmt.Errorf("get %q: %w", key, err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, 0, fmt.Errorf("get %q: stat: %w", key, err)
	}
	return f, info.Size(), nil
}

// Exists dit si un fichier existe sous la clé.
func (s *Store) Exists(_ context.Context, key string) (bool, error) {
	if err := checkKey(key); err != nil {
		return false, err
	}
	root, err := s.open()
	if err != nil {
		return false, err
	}
	defer func() { _ = root.Close() }()
	_, err = root.Stat(key)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("exists %q: %w", key, err)
	}
	return true, nil
}
