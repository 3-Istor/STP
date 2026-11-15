package viz3d

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// ModelView est un modèle tel que l'API le rend ; URL vide pour une primitive (pas de fichier).
type ModelView struct {
	ID         int64       `json:"id"`
	Name       string      `json:"name"`
	Source     string      `json:"source"`
	SourceURL  string      `json:"source_url"`
	Format     string      `json:"format"`
	License    string      `json:"license"`
	Dimensions *Dimensions `json:"dimensions"`
	Confidence string      `json:"confidence"`
	URL        string      `json:"url"`
	CreatedAt  time.Time   `json:"created_at"`
}

// PartModelView : le modèle d'une pièce (nil si elle n'en a pas) et ce qu'il vaut pour elle.
type PartModelView struct {
	PartID     int64      `json:"part_id"`
	Model      *ModelView `json:"model"`
	Match      MatchType  `json:"match_type,omitempty"`
	Confidence string     `json:"confidence,omitempty"`
}

func (s *Service) modelView(m Model) ModelView {
	v := ModelView{ID: m.ID, Name: m.Name, Source: m.Source, SourceURL: m.SourceURL, Format: m.Format, License: m.License,
		Dimensions: m.Dimensions, Confidence: m.Confidence.StringFixed(2), CreatedAt: m.CreatedAt}
	if m.Format == FormatGLB && s.modelURL != nil {
		v.URL = s.modelURL(m.ID)
	}
	return v
}

// PartModel renvoie le modèle d'une pièce ; catalog.ErrPartNotFound si la pièce n'existe pas.
func (s *Service) PartModel(ctx context.Context, partID int64) (PartModelView, error) {
	if s.store == nil {
		return PartModelView{}, ErrModelsUnavailable
	}
	if _, err := s.parts.Part(ctx, partID); err != nil {
		return PartModelView{}, fmt.Errorf("model of part %d: %w", partID, err)
	}
	r, err := s.store.PartModel(ctx, partID)
	if err != nil {
		if errors.Is(err, ErrNoModel) {
			return PartModelView{PartID: partID}, nil
		}
		return PartModelView{}, fmt.Errorf("model of part %d: %w", partID, err)
	}
	mv := s.modelView(r.Model)
	return PartModelView{PartID: partID, Model: &mv, Match: r.PartModel.Match, Confidence: r.PartModel.Confidence.StringFixed(2)}, nil
}

// Association décrit l'association d'un modèle existant à une pièce. Confidence nil : valeur par défaut du
// niveau (exact 1, similar 0.60, generated 0.20).
type Association struct {
	ModelID    int64
	Match      MatchType
	Confidence *decimal.Decimal
}

// defaultConfidence : ce qu'on suppose quand l'utilisateur ne dit rien ; toujours affiché, jamais caché.
func defaultConfidence(m MatchType) decimal.Decimal {
	switch m {
	case MatchExact:
		return decimal.NewFromInt(1)
	case MatchSimilar:
		return decimal.RequireFromString("0.60")
	default:
		return decimal.NewFromFloat(baseConfidence)
	}
}

func resolveConfidence(m MatchType, c *decimal.Decimal) (decimal.Decimal, error) {
	if !ValidMatch(m) {
		return decimal.Decimal{}, fmt.Errorf("match_type %q: %w", m, ErrInvalidModel)
	}
	if c == nil {
		return defaultConfidence(m), nil
	}
	if !ValidConfidence(*c) {
		return decimal.Decimal{}, fmt.Errorf("confidence %s is not in 0–1: %w", c, ErrInvalidModel)
	}
	return *c, nil
}

// AssignModel associe un modèle existant à une pièce (remplace l'association précédente).
func (s *Service) AssignModel(ctx context.Context, partID int64, a Association) (PartModelView, error) {
	if s.store == nil {
		return PartModelView{}, ErrModelsUnavailable
	}
	conf, err := resolveConfidence(a.Match, a.Confidence)
	if err != nil {
		return PartModelView{}, fmt.Errorf("assign model to part %d: %w", partID, err)
	}
	if _, err := s.parts.Part(ctx, partID); err != nil {
		return PartModelView{}, fmt.Errorf("assign model to part %d: %w", partID, err)
	}
	if _, err := s.store.Model(ctx, a.ModelID); err != nil {
		return PartModelView{}, fmt.Errorf("assign model to part %d: %w", partID, err)
	}
	if _, err := s.store.SetPartModel(ctx, PartModel{PartID: partID, ModelID: a.ModelID, Match: a.Match, Confidence: conf}); err != nil {
		return PartModelView{}, fmt.Errorf("assign model to part %d: %w", partID, err)
	}
	return s.PartModel(ctx, partID)
}

// Upload décrit un GLB téléversé pour une pièce. Data est lu jusqu'à MaxModelBytes + 1 : au-delà, refus.
type Upload struct {
	Filename   string
	Name       string
	License    string
	SourceURL  string
	Match      MatchType
	Confidence *decimal.Decimal
	Data       io.Reader
}

// UploadModel valide le GLB (extension, taille, en-tête glTF 2), le range dans le BlobStore sous une clé
// dérivée de son empreinte (deux téléversements identiques partagent le fichier), crée le modèle et
// l'associe à la pièce.
func (s *Service) UploadModel(ctx context.Context, partID int64, u Upload) (PartModelView, error) {
	if s.store == nil || s.blobs == nil {
		return PartModelView{}, ErrModelsUnavailable
	}
	conf, err := resolveConfidence(u.Match, u.Confidence)
	if err != nil {
		return PartModelView{}, fmt.Errorf("upload model for part %d: %w", partID, err)
	}
	if !strings.EqualFold(path.Ext(u.Filename), ".glb") {
		return PartModelView{}, fmt.Errorf("upload model for part %d: %q is not a .glb file: %w", partID, u.Filename, ErrInvalidModel)
	}
	if _, err := s.parts.Part(ctx, partID); err != nil {
		return PartModelView{}, fmt.Errorf("upload model for part %d: %w", partID, err)
	}
	data, err := io.ReadAll(io.LimitReader(u.Data, MaxModelBytes+1))
	if err != nil {
		return PartModelView{}, fmt.Errorf("upload model for part %d: read: %w", partID, err)
	}
	if len(data) > MaxModelBytes {
		return PartModelView{}, fmt.Errorf("upload model for part %d: file exceeds %d bytes: %w", partID, MaxModelBytes, ErrInvalidModel)
	}
	if err := ValidateGLB(data); err != nil {
		return PartModelView{}, fmt.Errorf("upload model for part %d: %w", partID, err)
	}
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	key := "models/" + hash[:2] + "/" + hash + ".glb"
	exists, err := s.blobs.Exists(ctx, key)
	if err != nil {
		return PartModelView{}, fmt.Errorf("upload model for part %d: %w", partID, err)
	}
	if !exists {
		if err := s.blobs.Put(ctx, key, bytes.NewReader(data)); err != nil {
			return PartModelView{}, fmt.Errorf("upload model for part %d: store file: %w", partID, err)
		}
	}
	name := strings.TrimSpace(u.Name)
	if name == "" {
		name = strings.TrimSuffix(path.Base(u.Filename), path.Ext(u.Filename))
	}
	m, err := s.store.CreateModel(ctx, Model{Name: name, Source: SourceUpload, SourceURL: u.SourceURL, StoragePath: key,
		Format: FormatGLB, License: u.License, Confidence: conf})
	if err != nil {
		return PartModelView{}, fmt.Errorf("upload model for part %d: %w", partID, err)
	}
	if _, err := s.store.SetPartModel(ctx, PartModel{PartID: partID, ModelID: m.ID, Match: u.Match, Confidence: conf}); err != nil {
		return PartModelView{}, fmt.Errorf("upload model for part %d: %w", partID, err)
	}
	return s.PartModel(ctx, partID)
}

// glTF binaire (GLB) : en-tête de 12 octets, magic « glTF », version 2, longueur totale du fichier.
const (
	glbMagic     = 0x46546C67
	glbVersion   = 2
	glbHeaderLen = 12
)

// ValidateGLB vérifie l'en-tête GLB : magic, version 2 et longueur annoncée égale à la taille lue.
// Une archive renommée en .glb ou un JSON .gltf sont refusés (ErrInvalidModel).
func ValidateGLB(data []byte) error {
	if len(data) < glbHeaderLen {
		return fmt.Errorf("file is shorter than a GLB header: %w", ErrInvalidModel)
	}
	if binary.LittleEndian.Uint32(data[0:4]) != glbMagic {
		return fmt.Errorf("missing glTF magic: %w", ErrInvalidModel)
	}
	if v := binary.LittleEndian.Uint32(data[4:8]); v != glbVersion {
		return fmt.Errorf("glTF version %d is not supported: %w", v, ErrInvalidModel)
	}
	if l := binary.LittleEndian.Uint32(data[8:12]); uint64(l) != uint64(len(data)) {
		return fmt.Errorf("GLB length %d does not match file size %d: %w", l, len(data), ErrInvalidModel)
	}
	return nil
}

// ModelFile ouvre le fichier d'un modèle ; ErrModelNotFound sans modèle, ErrNoModel pour une primitive.
func (s *Service) ModelFile(ctx context.Context, modelID int64) (Model, io.ReadCloser, int64, error) {
	if s.store == nil || s.blobs == nil {
		return Model{}, nil, 0, ErrModelsUnavailable
	}
	m, err := s.store.Model(ctx, modelID)
	if err != nil {
		return Model{}, nil, 0, fmt.Errorf("file of model %d: %w", modelID, err)
	}
	if m.Format != FormatGLB || m.StoragePath == "" {
		return Model{}, nil, 0, fmt.Errorf("file of model %d: primitive has no file: %w", modelID, ErrNoModel)
	}
	rc, size, err := s.blobs.Get(ctx, m.StoragePath)
	if err != nil {
		return Model{}, nil, 0, fmt.Errorf("file of model %d: %w", modelID, err)
	}
	return m, rc, size, nil
}
