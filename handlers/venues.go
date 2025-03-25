package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

type Venue struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Address     string `json:"address"`
	Description string `json:"description"`
	Slug        string `json:"slug"`
	LatLng      string `json:"latlng"`
	City        string `json:"city"`
}

func (h *AuthHandler) GetVenues(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Select(`SELECT id, name, address, description, slug, latlng, city FROM venues ORDER BY name ASC`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var venues []Venue
	for rows.Next() {
		var v Venue
		if err := rows.Scan(&v.ID, &v.Name, &v.Address, &v.Description, &v.Slug, &v.LatLng, &v.City); err != nil {
			continue
		}
		venues = append(venues, v)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(venues)
}
func (h *AuthHandler) GetVenueByIDOrSlug(w http.ResponseWriter, r *http.Request) {
	param := chi.URLParam(r, "id") // puede ser numérico o string
	var v Venue
	var row *sql.Row
	var err error

	if _, errConv := strconv.Atoi(param); errConv == nil {
		// Es un número → buscar por ID
		row, err = h.DB.SelectRow(`
			SELECT id, name, address, description, slug, latlng, city
			FROM venues WHERE id = ?`, param)
	} else {
		// No es número → buscar por slug
		row, err = h.DB.SelectRow(`
			SELECT id, name, address, description, slug, latlng, city
			FROM venues WHERE slug = ?`, param)
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = row.Scan(&v.ID, &v.Name, &v.Address, &v.Description, &v.Slug, &v.LatLng, &v.City)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func (h *AuthHandler) CreateVenue(w http.ResponseWriter, r *http.Request) {
	var v Venue
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		http.Error(w, "Error al decodificar", http.StatusBadRequest)
		return
	}

	id, err := h.DB.Insert(false, `
		INSERT INTO venues (name, address, description, slug, latlng, city)
		VALUES (?, ?, ?, ?, ?, ?)`,
		v.Name, v.Address, v.Description, v.Slug, v.LatLng, v.City)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	v.ID = int(id)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func (h *AuthHandler) UpdateVenue(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var v Venue
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		http.Error(w, "Error al decodificar", http.StatusBadRequest)
		return
	}

	_, err := h.DB.Update(false, `
		UPDATE venues SET name=?, address=?, description=?, slug=?, latlng=?, city=?
		WHERE id = ?`,
		v.Name, v.Address, v.Description, v.Slug, v.LatLng, v.City, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *AuthHandler) DeleteVenue(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	_, err := h.DB.Delete(false, "DELETE FROM venues WHERE id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
