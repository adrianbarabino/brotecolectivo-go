package handlers

import (
	"encoding/json"
	"fmt"
	"image"

	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"gopkg.in/ini.v1"
)

type Band struct {
	ID     int               `json:"id"`
	Name   string            `json:"name"`
	Bio    string            `json:"bio"`
	Slug   string            `json:"slug"`
	Social map[string]string `json:"social"`
}

func getBucket() string {
	cfg, _ := ini.Load("data.conf")
	return cfg.Section("spaces").Key("bucket").String()
}

func getEndpoint() string {
	cfg, _ := ini.Load("data.conf")
	return cfg.Section("spaces").Key("endpoint").String()
}
func (h *AuthHandler) CheckBandSlug(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		http.Error(w, "Falta el slug", http.StatusBadRequest)
		return
	}

	var exists bool
	row, _ := h.DB.SelectRow("SELECT EXISTS(SELECT 1 FROM bands WHERE slug = ?)", slug)
	err := row.Scan(&exists)
	if err != nil {
		http.Error(w, "Error al consultar la base de datos", http.StatusInternalServerError)
		return
	}

	if exists {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"exists": true}`))
	} else {
		http.Error(w, "Slug disponible", http.StatusNotFound)
	}
}

func (h *AuthHandler) GetBandsCount(w http.ResponseWriter, r *http.Request) {
	row, err := h.DB.SelectRow("SELECT COUNT(*) FROM bands")
	if err != nil {
		http.Error(w, "Error al contar artistas", http.StatusInternalServerError)
		return
	}

	var count int
	if err := row.Scan(&count); err != nil {
		http.Error(w, "Error al leer el conteo", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"count": count})
}
func (h *AuthHandler) UploadBandImage(w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(10 << 20) // 10MB max
	if err != nil {
		http.Error(w, "No se pudo procesar la imagen", http.StatusBadRequest)
		return
	}

	file, handler, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "No se pudo leer el archivo", http.StatusBadRequest)
		return
	}
	defer file.Close()

	slug := r.FormValue("slug")
	if slug == "" {
		http.Error(w, "Slug es requerido", http.StatusBadRequest)
		return
	}

	// Decodificamos la imagen
	srcImage, _, err := image.Decode(file)
	if err != nil {
		http.Error(w, "Formato de imagen no válido", http.StatusBadRequest)
		return
	}

	// Crear carpeta temporal si no existe
	if err := os.MkdirAll("tmp", os.ModePerm); err != nil {
		http.Error(w, "No se pudo crear la carpeta temporal", http.StatusInternalServerError)
		return
	}

	tmpFilePath := fmt.Sprintf("tmp/%s.jpg", slug)
	out, err := os.Create(tmpFilePath)
	if err != nil {
		http.Error(w, "No se pudo crear archivo temporal", http.StatusInternalServerError)
		return
	}

	defer out.Close()
	defer os.Remove(tmpFilePath) // limpia después de subir

	err = jpeg.Encode(out, srcImage, &jpeg.Options{Quality: 85})
	if err != nil {
		http.Error(w, "No se pudo convertir a JPG", http.StatusInternalServerError)
		return
	}

	// Subimos a Spaces
	key := fmt.Sprintf("bands/%s.jpg", slug)
	err = uploadToSpaces(tmpFilePath, key, "image/jpeg")
	if err != nil {
		http.Error(w, "Error al subir a Spaces: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Respuesta con éxito
	url := fmt.Sprintf("https://%s.%s/%s", getBucket(), getEndpoint(), key)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
		"url":    url,
		"name":   handler.Filename,
	})
}

func (h *AuthHandler) GetBandsDatatable(w http.ResponseWriter, r *http.Request) {
	offsetParam := r.URL.Query().Get("offset")
	limitParam := r.URL.Query().Get("limit")
	search := r.URL.Query().Get("q")
	sortBy := r.URL.Query().Get("sort")
	order := r.URL.Query().Get("order")

	idFilter := r.URL.Query().Get("id")
	nameFilter := r.URL.Query().Get("name")
	slugFilter := r.URL.Query().Get("slug")

	offset := 0
	limit := 10
	var err error

	if offsetParam != "" {
		offset, err = strconv.Atoi(offsetParam)
		if err != nil {
			http.Error(w, "Offset inválido", http.StatusBadRequest)
			return
		}
	}
	if limitParam != "" {
		limit, err = strconv.Atoi(limitParam)
		if err != nil {
			http.Error(w, "Límite inválido", http.StatusBadRequest)
			return
		}
	}

	query := "SELECT id, name, bio, slug, social FROM bands WHERE 1=1"
	var queryParams []interface{}

	if search != "" {
		pattern := "%" + search + "%"
		query += " AND (name LIKE ? OR slug LIKE ? OR bio LIKE ?)"
		queryParams = append(queryParams, pattern, pattern, pattern)
	}
	if idFilter != "" {
		query += " AND id LIKE ?"
		queryParams = append(queryParams, "%"+idFilter+"%")
	}
	if nameFilter != "" {
		query += " AND name LIKE ?"
		queryParams = append(queryParams, "%"+nameFilter+"%")
	}
	if slugFilter != "" {
		query += " AND slug LIKE ?"
		queryParams = append(queryParams, "%"+slugFilter+"%")
	}

	if sortBy != "" {
		validSorts := map[string]bool{"id": true, "name": true, "slug": true}
		if validSorts[sortBy] {
			if order != "desc" {
				order = "asc"
			}
			query += fmt.Sprintf(" ORDER BY %s %s", sortBy, strings.ToUpper(order))
		}
	}

	query += " LIMIT ? OFFSET ?"
	queryParams = append(queryParams, limit, offset)

	rows, err := h.DB.Select(query, queryParams...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var bands []Band
	for rows.Next() {
		var b Band
		var socialRaw []byte

		if err := rows.Scan(&b.ID, &b.Name, &b.Bio, &b.Slug, &socialRaw); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if err := json.Unmarshal(socialRaw, &b.Social); err != nil {
			b.Social = map[string]string{}
		}

		bands = append(bands, b)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(bands)
}

func (h *AuthHandler) GetBands(w http.ResponseWriter, r *http.Request) {
	offsetParam := r.URL.Query().Get("offset")
	limitParam := r.URL.Query().Get("limit")
	search := r.URL.Query().Get("q")
	sortBy := r.URL.Query().Get("sort")
	order := r.URL.Query().Get("order")

	offset := 0
	limit := 10
	var err error

	if offsetParam != "" {
		offset, err = strconv.Atoi(offsetParam)
		if err != nil {
			http.Error(w, "Offset inválido", http.StatusBadRequest)
			return
		}
	}

	if limitParam != "" {
		limit, err = strconv.Atoi(limitParam)
		if err != nil {
			http.Error(w, "Límite inválido", http.StatusBadRequest)
			return
		}
	}

	query := "SELECT id, name, bio, slug, social FROM bands WHERE 1=1"
	var queryParams []interface{}

	if search != "" {
		query += " AND (name LIKE ? OR bio LIKE ? OR slug LIKE ? OR social LIKE ?)"
		searchPattern := "%" + search + "%"
		queryParams = append(queryParams, searchPattern, searchPattern, searchPattern, searchPattern)
	}

	if sortBy != "" {
		validSorts := map[string]bool{"id": true, "name": true, "slug": true}
		if validSorts[sortBy] {
			if order != "desc" {
				order = "asc"
			}
			query += fmt.Sprintf(" ORDER BY %s %s", sortBy, strings.ToUpper(order))
		}
	}

	query += " LIMIT ? OFFSET ?"
	queryParams = append(queryParams, limit, offset)

	rows, err := h.DB.Select(query, queryParams...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var bands []Band
	for rows.Next() {
		var b Band
		var socialRaw []byte

		if err := rows.Scan(&b.ID, &b.Name, &b.Bio, &b.Slug, &socialRaw); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if err := json.Unmarshal(socialRaw, &b.Social); err != nil {

			// si falla, igual devolvemos un map vacío
			b.Social = map[string]string{}
		}

		bands = append(bands, b)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(bands)
}

func (h *AuthHandler) GetBandByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var b Band
	var socialRaw []byte

	// allow to id could be a slug
	if _, err := strconv.Atoi(id); err != nil {
		// es un slug, buscamos por slug
		row, err := h.DB.SelectRow("SELECT id, name, bio, slug, social FROM bands WHERE slug = ?", id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		err = row.Scan(&b.ID, &b.Name, &b.Bio, &b.Slug, &socialRaw)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
	} else {

		row, err := h.DB.SelectRow("SELECT id, name, bio, slug, social FROM bands WHERE id = ?", id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		err = row.Scan(&b.ID, &b.Name, &b.Bio, &b.Slug, &socialRaw)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
	}

	if err := json.Unmarshal(socialRaw, &b.Social); err != nil {
		b.Social = map[string]string{}
	}
	json.NewEncoder(w).Encode(b)
}

func (h *AuthHandler) CreateBand(w http.ResponseWriter, r *http.Request) {
	var b Band
	json.NewDecoder(r.Body).Decode(&b)
	socialJSON, _ := json.Marshal(b.Social)
	id, err := h.DB.Insert(false, "INSERT INTO bands (name, bio, slug, social) VALUES (?, ?, ?, ?)", b.Name, b.Bio, b.Slug, string(socialJSON))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	b.ID = int(id)
	json.NewEncoder(w).Encode(b)
}

func (h *AuthHandler) UpdateBand(w http.ResponseWriter, r *http.Request) {
	var b Band
	id := chi.URLParam(r, "id")

	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		http.Error(w, "Error al decodificar el cuerpo", http.StatusBadRequest)
		return
	}

	socialJSON, _ := json.Marshal(b.Social)

	_, err := h.DB.Update(false, "UPDATE bands SET name=?, bio=?, slug=?, social=? WHERE id=?", b.Name, b.Bio, b.Slug, string(socialJSON), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *AuthHandler) DeleteBand(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	_, err := h.DB.Delete(false, "DELETE FROM bands WHERE id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
