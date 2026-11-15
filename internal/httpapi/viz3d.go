package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"

	"bomstudio/internal/viz3d"
)

// AssemblyService est ce que l'API attend du domaine viz3d ; *viz3d.Service la satisfait (ADR 0031).
type AssemblyService interface {
	Assembly(ctx context.Context, bomID int64) (viz3d.Assembly, error)
	GenerationStatus(ctx context.Context, bomID int64) (viz3d.GenerationView, error)
	StartGeneration(ctx context.Context, bomID int64) (viz3d.GenerationView, error)
	PartModel(ctx context.Context, partID int64) (viz3d.PartModelView, error)
	AssignModel(ctx context.Context, partID int64, a viz3d.Association) (viz3d.PartModelView, error)
	UploadModel(ctx context.Context, partID int64, u viz3d.Upload) (viz3d.PartModelView, error)
	ModelFile(ctx context.Context, modelID int64) (viz3d.Model, io.ReadCloser, int64, error)
}

var _ AssemblyService = (*viz3d.Service)(nil)

// ModelFileURL est l'URL publique du fichier d'un modèle ; le câblage la passe au domaine, qui ne connaît
// pas les routes.
func ModelFileURL(modelID int64) string { return "/api/v1/models/" + strconv.FormatInt(modelID, 10) + "/file" }

// Les fichiers GLB sont lus en mémoire par le domaine (MaxModelBytes) ; le formulaire multipart garde un
// peu de marge pour les champs texte.
const maxUploadBytes = viz3d.MaxModelBytes + (64 << 10)

type viz3dHandler struct {
	service AssemblyService
}

func (h viz3dHandler) register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/boms/{id}/3d", h.assembly)
	mux.HandleFunc("GET /api/v1/boms/{id}/3d/status", h.status)
	mux.HandleFunc("POST /api/v1/boms/{id}/3d/generate", h.generate)
	mux.HandleFunc("GET /api/v1/parts/{id}/3d", h.partModel)
	mux.HandleFunc("PUT /api/v1/parts/{id}/3d", h.putPartModel)
	mux.HandleFunc("GET /api/v1/models/{id}/file", h.modelFile)
}

// assembly renvoie la scène 3D d'un brouillon : décoder l'id, appeler le domaine, encoder.
func (h viz3dHandler) assembly(w http.ResponseWriter, r *http.Request) {
	id, err := bomPathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	a, err := h.service.Assembly(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (h viz3dHandler) status(w http.ResponseWriter, r *http.Request) {
	id, err := bomPathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	g, err := h.service.GenerationStatus(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}

// generate lance la génération en tâche de fond : 202 avec l'état « queued » ; 409 si déjà en cours.
func (h viz3dHandler) generate(w http.ResponseWriter, r *http.Request) {
	id, err := bomPathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	g, err := h.service.StartGeneration(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, g)
}

func (h viz3dHandler) partModel(w http.ResponseWriter, r *http.Request) {
	id, err := bomPathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	v, err := h.service.PartModel(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// assignRequest : association d'un modèle existant (corps JSON).
type assignRequest struct {
	ModelID    int64   `json:"model_id"`
	MatchType  string  `json:"match_type"`
	Confidence *string `json:"confidence"`
}

// putPartModel associe un modèle à une pièce : corps JSON {model_id, match_type, confidence} pour un modèle
// existant, ou multipart/form-data avec le champ `file` (GLB) et les champs `match_type`, `confidence`,
// `name`, `license`, `source_url` facultatifs.
func (h viz3dHandler) putPartModel(w http.ResponseWriter, r *http.Request) {
	id, err := bomPathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	var v viz3d.PartModelView
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		v, err = h.upload(w, r, id)
	} else {
		var req assignRequest
		if err = decode(r, &req); err != nil {
			writeError(w, r, err)
			return
		}
		var conf *decimal.Decimal
		if conf, err = optionalDecimal(req.Confidence); err != nil {
			writeError(w, r, err)
			return
		}
		v, err = h.service.AssignModel(r.Context(), id, viz3d.Association{ModelID: req.ModelID, Match: viz3d.MatchType(req.MatchType), Confidence: conf})
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h viz3dHandler) upload(w http.ResponseWriter, r *http.Request, partID int64) (viz3d.PartModelView, error) {
	// Corps borné (MaxBytesReader) : le GLB lui-même est relu et borné par le domaine. Les parties plus
	// grandes que le seuil vont sur disque temporaire.
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(1 << 20); err != nil { //nolint:gosec // G120 : corps déjà borné juste au-dessus
		return viz3d.PartModelView{}, fmt.Errorf("multipart form: %w: %w", err, errBadRequest)
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()
	f, header, err := r.FormFile("file")
	if err != nil {
		return viz3d.PartModelView{}, fmt.Errorf("multipart field \"file\" is required: %w", errBadRequest)
	}
	defer func() { _ = f.Close() }()
	match := viz3d.MatchType(r.FormValue("match_type"))
	if match == "" {
		match = viz3d.MatchExact
	}
	var confRaw *string
	if c := r.FormValue("confidence"); c != "" {
		confRaw = &c
	}
	conf, err := optionalDecimal(confRaw)
	if err != nil {
		return viz3d.PartModelView{}, err
	}
	return h.service.UploadModel(r.Context(), partID, viz3d.Upload{
		Filename: header.Filename, Name: r.FormValue("name"), License: r.FormValue("license"),
		SourceURL: r.FormValue("source_url"), Match: match, Confidence: conf, Data: f,
	})
}

func optionalDecimal(raw *string) (*decimal.Decimal, error) {
	if raw == nil {
		return nil, nil
	}
	d, err := decimal.NewFromString(*raw)
	if err != nil {
		return nil, fmt.Errorf("confidence %q is not a decimal: %w", *raw, errBadRequest)
	}
	return &d, nil
}

// modelFile sert le GLB d'un modèle. Le contenu est immuable (clé par empreinte) : cache long.
func (h viz3dHandler) modelFile(w http.ResponseWriter, r *http.Request) {
	id, err := bomPathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	m, rc, size, err := h.service.ModelFile(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	defer func() { _ = rc.Close() }()
	w.Header().Set("Content-Type", "model/gltf-binary")
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	w.Header().Set("Content-Disposition", `inline; filename="`+strconv.FormatInt(m.ID, 10)+`.glb"`)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, rc); err != nil && !errors.Is(err, context.Canceled) {
		// Les en-têtes sont partis : rien d'autre à faire que le dire.
		writeErrorLog(r, err)
	}
}
