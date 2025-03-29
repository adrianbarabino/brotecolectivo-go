package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"brotecolectivo/models"

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

	// Obtener el ID del usuario autenticado
	claims, ok := r.Context().Value("user").(*models.Claims)
	if !ok {
		http.Error(w, "Usuario no autenticado", http.StatusUnauthorized)
		return
	}
	userID := claims.UserID

	id, err := h.DB.Insert(false, `
		INSERT INTO venues (name, address, description, slug, latlng, city)
		VALUES (?, ?, ?, ?, ?, ?)`,
		v.Name, v.Address, v.Description, v.Slug, v.LatLng, v.City)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	v.ID = int(id)

	// Vincular el venue con el usuario que lo creó
	_, linkErr := h.DB.Insert(false, `
		INSERT INTO venue_links (user_id, venue_id, rol, status) 
		VALUES (?, ?, 'owner', 'approved')`,
		userID, id)
	if linkErr != nil {
		fmt.Printf("Error al vincular venue %d con usuario %d: %v\n", id, userID, linkErr)
	}

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

// GetUserVenues obtiene los venues (espacios culturales) vinculados a un usuario específico.
//
// @Summary Obtener venues de un usuario
// @Description Obtiene los venues vinculados a un usuario específico
// @Tags venues
// @Accept json
// @Produce json
// @Param user_id path int true "ID del usuario"
// @Success 200 {array} Venue "Lista de venues vinculados al usuario"
// @Failure 400 {string} string "ID inválido"
// @Failure 500 {string} string "Error al obtener los venues"
// @Security BearerAuth
// @Router /venues/user/{user_id} [get]
func (h *AuthHandler) GetUserVenues(w http.ResponseWriter, r *http.Request) {
	// Configurar encabezados para JSON
	w.Header().Set("Content-Type", "application/json")

	userID := chi.URLParam(r, "user_id")

	// Verificar que el usuario autenticado tenga permiso para ver estos venues
	claims, ok := r.Context().Value("user").(*models.Claims)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Usuario no autenticado"})
		return
	}

	// Convertir userID de string a uint para comparar con claims.UserID
	userIDUint, err := strconv.ParseUint(userID, 10, 32)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "ID de usuario inválido"})
		return
	}

	// Solo permitir acceso si es el mismo usuario o es admin
	if claims.UserID != uint(userIDUint) && claims.Role != "admin" {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "No tienes permiso para ver estos venues"})
		return
	}

	// Consultar los venues vinculados al usuario
	rows, err := h.DB.Select(`
		SELECT v.id, v.name, v.address, v.description, v.slug, v.latlng, v.city, vl.rol
		FROM venues v
		JOIN venue_links vl ON v.id = vl.venue_id
		WHERE vl.user_id = ? AND vl.status = 'approved'
		ORDER BY v.name ASC`, userID)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al consultar los venues: " + err.Error()})
		return
	}
	defer rows.Close()

	venues := []Venue{}

	for rows.Next() {
		var venue Venue
		var rol string

		if err := rows.Scan(&venue.ID, &venue.Name, &venue.Address, &venue.Description,
			&venue.Slug, &venue.LatLng, &venue.City, &rol); err != nil {
			continue // Saltamos este registro si hay error
		}

		venues = append(venues, venue)
	}

	json.NewEncoder(w).Encode(venues)
}
