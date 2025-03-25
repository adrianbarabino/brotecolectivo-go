package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

type Band struct {
	ID     int               `json:"id"`
	Name   string            `json:"name"`
	Bio    string            `json:"bio"`
	Slug   string            `json:"slug"`
	Social map[string]string `json:"social"`
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
