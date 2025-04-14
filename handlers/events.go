package handlers

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"brotecolectivo/models"

	"github.com/go-chi/chi/v5"
	"gopkg.in/ini.v1"

	drawImage "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Event representa un evento cultural en la plataforma Brote Colectivo.
// Contiene información sobre fechas, ubicación, artistas participantes y detalles del evento.
//
// @Schema
type Event struct {
	ID        int    `json:"id"`
	Title     string `json:"title"`
	Tags      string `json:"tags"`
	Content   string `json:"content"`
	Slug      string `json:"slug"`
	DateStart string `json:"date_start"`
	DateEnd   string `json:"date_end"`
	Venue     *Venue `json:"venue"`
	Bands     []Band `json:"bands"`
	VenueID   int    `json:"id_venue"` // <--- agregar esto
	Rol       string `json:"rol"`
}

// GetEventsCount devuelve el número total de eventos en la base de datos.
//
// @Summary Obtener conteo de eventos
// @Description Devuelve el número total de eventos registrados en la plataforma
// @Tags eventos
// @Produce json
// @Success 200 {object} map[string]int "Conteo exitoso"
// @Failure 500 {string} string "Error interno del servidor"
// @Router /events/count [get]
func (h *AuthHandler) GetEventsCount(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("q")
	idFilter := r.URL.Query().Get("id")
	titleFilter := r.URL.Query().Get("title")
	dateFilter := r.URL.Query().Get("date_start")

	query := `
		SELECT COUNT(*) 
		FROM events e
		JOIN venues v ON e.id_venue = v.id
		WHERE 1=1
	`
	var queryParams []interface{}

	if search != "" {
		pattern := "%" + search + "%"
		query += ` AND (e.title LIKE ? OR e.tags LIKE ? OR e.content LIKE ? OR e.slug LIKE ?)`
		queryParams = append(queryParams, pattern, pattern, pattern, pattern)
	}
	if idFilter != "" {
		query += " AND e.id LIKE ?"
		queryParams = append(queryParams, "%"+idFilter+"%")
	}
	if titleFilter != "" {
		query += " AND e.title LIKE ?"
		queryParams = append(queryParams, "%"+titleFilter+"%")
	}
	if dateFilter != "" {
		query += " AND e.date_start LIKE ?"
		queryParams = append(queryParams, "%"+dateFilter+"%")
	}

	row, err := h.DB.SelectRow(query, queryParams...)
	if err != nil {
		http.Error(w, "Error al contar eventos", http.StatusInternalServerError)
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

// GetEventsDatatable devuelve los datos de eventos en formato para DataTables.
//
// @Summary Obtener eventos para DataTables
// @Description Devuelve los datos de eventos formateados para su uso con DataTables
// @Tags eventos
// @Produce json
// @Param draw query int false "Parámetro draw de DataTables"
// @Param start query int false "Índice de inicio para paginación"
// @Param length query int false "Número de registros a mostrar"
// @Param search[value] query string false "Término de búsqueda"
// @Param order[0][column] query int false "Índice de la columna para ordenar"
// @Param order[0][dir] query string false "Dirección de ordenamiento (asc/desc)"
// @Success 200 {object} DatatableResponse "Datos para DataTables"
// @Failure 500 {string} string "Error interno del servidor"
// @Router /events/table [get]
func (h *AuthHandler) GetEventsDatatable(w http.ResponseWriter, r *http.Request) {
	offsetParam := r.URL.Query().Get("offset")
	limitParam := r.URL.Query().Get("limit")
	search := r.URL.Query().Get("q")
	sortBy := r.URL.Query().Get("sort")
	order := r.URL.Query().Get("order")

	idFilter := r.URL.Query().Get("id")
	titleFilter := r.URL.Query().Get("title")
	dateFilter := r.URL.Query().Get("date_start")

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

	query := `
		SELECT 
			e.id, e.title, e.tags, e.content, e.slug, e.date_start, e.date_end,
			v.id, v.name
		FROM events e
		JOIN venues v ON e.id_venue = v.id
		WHERE 1=1
	`
	var queryParams []interface{}

	if search != "" {
		pattern := "%" + search + "%"
		query += ` AND (e.title LIKE ? OR e.tags LIKE ? OR e.content LIKE ? OR e.slug LIKE ?)`
		queryParams = append(queryParams, pattern, pattern, pattern, pattern)
	}
	if idFilter != "" {
		query += " AND e.id LIKE ?"
		queryParams = append(queryParams, "%"+idFilter+"%")
	}
	if titleFilter != "" {
		query += " AND e.title LIKE ?"
		queryParams = append(queryParams, "%"+titleFilter+"%")
	}
	if dateFilter != "" {
		query += " AND e.date_start LIKE ?"
		queryParams = append(queryParams, "%"+dateFilter+"%")
	}

	if sortBy != "" {
		validSorts := map[string]bool{"id": true, "title": true, "date_start": true}
		if validSorts[sortBy] {
			if order != "desc" {
				order = "asc"
			}
			query += fmt.Sprintf(" ORDER BY e.%s %s", sortBy, strings.ToUpper(order))
		}
	}

	query += " LIMIT ? OFFSET ?"
	queryParams = append(queryParams, limit, offset)
	fmt.Println("QUERY:", query)
	fmt.Println("PARAMS:", queryParams)

	rows, err := h.DB.Select(query, queryParams...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		var v Venue
		err := rows.Scan(&e.ID, &e.Title, &e.Tags, &e.Content, &e.Slug, &e.DateStart, &e.DateEnd,
			&v.ID, &v.Name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		e.Venue = &v

		bandRows, err := h.DB.Select(`
			SELECT b.id, b.name, b.slug
			FROM bands b
			JOIN events_bands eb ON b.id = eb.id_band
			WHERE eb.id_event = ?
		`, e.ID)
		if err == nil {
			var bands []Band
			for bandRows.Next() {
				var b Band
				if err := bandRows.Scan(&b.ID, &b.Name, &b.Slug); err == nil {
					bands = append(bands, b)
				}
			}
			bandRows.Close()
			e.Bands = bands
		}

		events = append(events, e)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events)
}

// GetEvents devuelve todos los eventos, con opciones de filtrado y paginación.
//
// @Summary Listar eventos
// @Description Obtiene una lista de eventos con opciones de filtrado y paginación
// @Tags eventos
// @Produce json
// @Param page query int false "Número de página para paginación"
// @Param limit query int false "Límite de registros por página"
// @Param search query string false "Término de búsqueda"
// @Param upcoming query bool false "Solo eventos futuros"
// @Param past query bool false "Solo eventos pasados"
// @Success 200 {array} Event "Lista de eventos"
// @Failure 500 {string} string "Error interno del servidor"
// @Router /events [get]
func (h *AuthHandler) GetEvents(w http.ResponseWriter, r *http.Request) {
	offsetParam := r.URL.Query().Get("offset")
	limitParam := r.URL.Query().Get("limit")
	search := r.URL.Query().Get("q")

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

	query := `
		SELECT 
			e.id, e.title, e.tags, e.content, e.slug, e.date_start, e.date_end,
			v.id, v.name
		FROM events e
		JOIN venues v ON e.id_venue = v.id
		WHERE 1=1
	`
	var queryParams []interface{}

	if search != "" {
		query += " AND (e.title LIKE ? OR e.tags LIKE ? OR e.content LIKE ? OR e.slug LIKE ?)"
		searchPattern := "%" + search + "%"
		queryParams = append(queryParams, searchPattern, searchPattern, searchPattern, searchPattern)
	}

	query += " ORDER BY e.date_start DESC LIMIT ? OFFSET ?"
	queryParams = append(queryParams, limit, offset)

	rows, err := h.DB.Select(query, queryParams...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var events []Event

	for rows.Next() {
		var e Event
		var v Venue
		err := rows.Scan(&e.ID, &e.Title, &e.Tags, &e.Content, &e.Slug, &e.DateStart, &e.DateEnd,
			&v.ID, &v.Name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		e.Venue = &v

		// Bandas asociadas
		bandRows, err := h.DB.Select(`
			SELECT b.id, b.name, b.slug
			FROM bands b
			JOIN events_bands eb ON b.id = eb.id_band
			WHERE eb.id_event = ?
		`, e.ID)
		if err == nil {
			var bands []Band
			for bandRows.Next() {
				var b Band
				if err := bandRows.Scan(&b.ID, &b.Name, &b.Slug); err == nil {
					bands = append(bands, b)
				}
			}
			bandRows.Close()
			e.Bands = bands
		}

		events = append(events, e)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events)
}

// GetEventByID devuelve un evento específico por su ID.
//
// @Summary Obtener evento por ID
// @Description Obtiene los detalles completos de un evento específico por su ID
// @Tags eventos
// @Produce json
// @Param id path int true "ID del evento"
// @Success 200 {object} Event "Detalles del evento"
// @Failure 400 {string} string "Error en la solicitud"
// @Failure 404 {string} string "Evento no encontrado"
// @Failure 500 {string} string "Error interno del servidor"
// @Router /events/{id} [get]
func (h *AuthHandler) GetEventByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var e Event
	var v Venue

	// check if id is numeric or is a slug
	var row *sql.Row
	var err error
	if _, errConv := strconv.Atoi(id); errConv == nil {
		// Es un número → buscar por ID
		row, _ = h.DB.SelectRow(`
			SELECT
				e.id, e.title, e.tags, e.content, e.slug, e.date_start, e.date_end,
				v.id, v.name, v.latlng, v.address, v.city
			FROM events e
			JOIN venues v ON e.id_venue = v.id
			WHERE e.id = ?`,
			id)
	} else {
		// No es número → buscar por slug
		row, _ = h.DB.SelectRow(`
			SELECT
				e.id, e.title, e.tags, e.content, e.slug, e.date_start, e.date_end,
				v.id, v.name, v.latlng, v.address, v.city
			FROM events e
			JOIN venues v ON e.id_venue = v.id
			WHERE e.slug = ?`,
			id)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = row.Scan(&e.ID, &e.Title, &e.Tags, &e.Content, &e.Slug, &e.DateStart, &e.DateEnd,
		&v.ID, &v.Name, &v.LatLng, &v.Address, &v.City)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	e.Venue = &v

	// Bandas
	bandRows, err := h.DB.Select(`
		SELECT b.id, b.name, b.slug
		FROM bands b
		JOIN events_bands eb ON b.id = eb.id_band
		WHERE eb.id_event = ?
	`, e.ID)
	if err == nil {
		defer bandRows.Close()
		for bandRows.Next() {
			var b Band
			if err := bandRows.Scan(&b.ID, &b.Name, &b.Slug); err == nil {
				e.Bands = append(e.Bands, b)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(e)
}

// GetEventsByVenueID devuelve todos los eventos asociados a un venue específico.
//
// @Summary Obtener eventos por venue
// @Description Obtiene todos los eventos que se realizan en un venue específico
// @Tags eventos
// @Produce json
// @Param id path int true "ID del venue"
// @Success 200 {array} Event "Lista de eventos del venue"
// @Failure 400 {string} string "Error en la solicitud"
// @Failure 404 {string} string "Venue no encontrado"
// @Failure 500 {string} string "Error interno del servidor"
// @Router /events/venue/{id} [get]
func (h *AuthHandler) GetEventsByVenueID(w http.ResponseWriter, r *http.Request) {
	venueID := chi.URLParam(r, "id")
	query := `
		SELECT 
			e.id, e.title, e.tags, e.content, e.slug, e.date_start, e.date_end,
			v.id, v.name
		FROM events e
		JOIN venues v ON e.id_venue = v.id
		WHERE e.id_venue = ?
		ORDER BY e.date_start DESC
	`

	rows, err := h.DB.Select(query, venueID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		var v Venue
		if err := rows.Scan(&e.ID, &e.Title, &e.Tags, &e.Content, &e.Slug, &e.DateStart, &e.DateEnd,
			&v.ID, &v.Name); err != nil {
			continue
		}
		e.Venue = &v

		// Bandas asociadas
		bandRows, _ := h.DB.Select(`
			SELECT b.id, b.name, b.slug
			FROM bands b
			JOIN events_bands eb ON b.id = eb.id_band
			WHERE eb.id_event = ?
		`, e.ID)
		defer bandRows.Close()
		for bandRows.Next() {
			var b Band
			if err := bandRows.Scan(&b.ID, &b.Name, &b.Slug); err == nil {
				e.Bands = append(e.Bands, b)
			}
		}

		events = append(events, e)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events)
}

// GetEventsByBandID devuelve todos los eventos asociados a una banda específica.
//
// @Summary Obtener eventos por banda
// @Description Obtiene todos los eventos en los que participa una banda específica
// @Tags eventos
// @Produce json
// @Param id path int true "ID de la banda"
// @Success 200 {array} Event "Lista de eventos de la banda"
// @Failure 400 {string} string "Error en la solicitud"
// @Failure 404 {string} string "Banda no encontrada"
// @Failure 500 {string} string "Error interno del servidor"
// @Router /events/band/{id} [get]
func (h *AuthHandler) GetEventsByBandID(w http.ResponseWriter, r *http.Request) {
	bandID := chi.URLParam(r, "id")
	query := `
		SELECT 
			e.id, e.title, e.tags, e.content, e.slug, e.date_start, e.date_end,
			v.id, v.name
		FROM events e
		JOIN events_bands eb ON e.id = eb.id_event
		JOIN venues v ON e.id_venue = v.id
		WHERE eb.id_band = ?
		ORDER BY e.date_start DESC
	`

	rows, err := h.DB.Select(query, bandID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		var v Venue
		if err := rows.Scan(&e.ID, &e.Title, &e.Tags, &e.Content, &e.Slug, &e.DateStart, &e.DateEnd,
			&v.ID, &v.Name); err != nil {
			continue
		}
		e.Venue = &v

		// Bandas asociadas
		bandRows, _ := h.DB.Select(`
			SELECT b.id, b.name, b.slug
			FROM bands b
			JOIN events_bands eb ON b.id = eb.id_band
			WHERE eb.id_event = ?
		`, e.ID)
		defer bandRows.Close()
		for bandRows.Next() {
			var b Band
			if err := bandRows.Scan(&b.ID, &b.Name, &b.Slug); err == nil {
				e.Bands = append(e.Bands, b)
			}
		}

		events = append(events, e)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events)
}

// GetEventBands devuelve todas las bandas asociadas a un evento específico.
//
// @Summary Obtener bandas de un evento
// @Description Obtiene todas las bandas que participan en un evento específico
// @Tags eventos
// @Produce json
// @Param id path int true "ID del evento"
// @Success 200 {array} Band "Lista de bandas del evento"
// @Failure 400 {string} string "Error en la solicitud"
// @Failure 404 {string} string "Evento no encontrado"
// @Failure 500 {string} string "Error interno del servidor"
// @Router /events/{id}/bands [get]
func (h *AuthHandler) GetEventBands(w http.ResponseWriter, r *http.Request) {
	eventID := chi.URLParam(r, "id")

	// Verificar que el evento existe
	var exists bool
	row, _ := h.DB.SelectRow("SELECT EXISTS(SELECT 1 FROM events WHERE id = ?)", eventID)
	if err := row.Scan(&exists); err != nil {
		http.Error(w, "Error al verificar evento: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if !exists {
		http.Error(w, "Evento no encontrado", http.StatusNotFound)
		return
	}

	// Obtener bandas asociadas al evento
	query := `
		SELECT b.id, b.name, b.bio, b.slug
		FROM bands b
		JOIN events_bands eb ON b.id = eb.id_band
		WHERE eb.id_event = ?
	`

	rows, err := h.DB.Select(query, eventID)
	if err != nil {
		http.Error(w, "Error al obtener bandas: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var bands []Band
	for rows.Next() {
		var b Band

		if err := rows.Scan(&b.ID, &b.Name, &b.Bio, &b.Slug); err != nil {
			http.Error(w, "Error al escanear bandas: "+err.Error(), http.StatusInternalServerError)
			return
		}

		bands = append(bands, b)
	}

	// Si no hay bandas, devolver array vacío en lugar de null
	if bands == nil {
		bands = []Band{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(bands)
}

// UploadEventImage maneja la subida de imágenes para eventos.
//
// @Summary Subir imagen de evento
// @Description Sube una imagen para un evento y la almacena en DigitalOcean Spaces
// @Tags eventos
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "Archivo de imagen a subir"
// @Param slug formData string true "Slug del evento"
// @Security BearerAuth
// @Success 200 {object} map[string]string "URL de la imagen subida"
// @Failure 400 {string} string "Error en la solicitud"
// @Failure 401 {string} string "No autorizado"
// @Failure 500 {string} string "Error interno del servidor"
// @Router /events/upload-image [post]
func (h *AuthHandler) UploadEventImage(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(10 << 20)
	file, handler, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "No se pudo leer el archivo", http.StatusBadRequest)
		return
	}
	defer file.Close()

	slug := r.FormValue("slug")
	if slug == "" {
		http.Error(w, "Slug faltante", http.StatusBadRequest)
		return
	}

	tempFile, err := os.CreateTemp("", "upload-*.jpg")
	if err != nil {
		http.Error(w, "No se pudo crear archivo temporal", http.StatusInternalServerError)
		return
	}
	defer os.Remove(tempFile.Name())
	io.Copy(tempFile, file)

	err = uploadToSpaces(tempFile.Name(), "events/"+slug+".jpg", handler.Header.Get("Content-Type"))
	if err != nil {
		http.Error(w, "Error al subir imagen: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"message": "Imagen subida con éxito"})
}

// CreateEvent crea un nuevo evento en la base de datos.
//
// @Summary Crear evento
// @Description Crea un nuevo evento con la información proporcionada
// @Tags eventos
// @Accept json
// @Produce json
// @Param event body Event true "Datos del evento a crear"
// @Security BearerAuth
// @Success 201 {object} Event "Evento creado exitosamente"
// @Failure 400 {string} string "Error en la solicitud"
// @Failure 401 {string} string "No autorizado"
// @Failure 500 {string} string "Error interno del servidor"
// @Router /events [post]
func (h *AuthHandler) CreateEvent(w http.ResponseWriter, r *http.Request) {
	type EventInput struct {
		IDVenue   int    `json:"id_venue"`
		Title     string `json:"title"`
		Tags      string `json:"tags"`
		Content   string `json:"content"`
		Slug      string `json:"slug"`
		DateStart string `json:"date_start"`
		DateEnd   string `json:"date_end"`
		BandIDs   []int  `json:"band_ids"` // <-- Lista de bandas
	}

	var input EventInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "Error al decodificar el cuerpo", http.StatusBadRequest)
		return
	}

	// Obtener el ID del usuario autenticado
	claims, ok := r.Context().Value("user").(*models.Claims)
	if !ok {
		http.Error(w, "Usuario no autenticado", http.StatusUnauthorized)
		return
	}
	userID := claims.UserID

	// Insertar el evento
	eventID, err := h.DB.Insert(false, `
		INSERT INTO events (id_venue, title, tags, content, slug, date_start, date_end)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		input.IDVenue, input.Title, input.Tags, input.Content, input.Slug, input.DateStart, input.DateEnd)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Insertar relaciones en events_bands
	for _, bandID := range input.BandIDs {
		_, err := h.DB.Insert(false, `
			INSERT INTO events_bands (id_band, id_event) VALUES (?, ?)`,
			bandID, eventID)
		if err != nil {
			// No detiene el proceso si falla una banda
			fmt.Printf("Error insertando banda %d: %v\n", bandID, err)
		}
	}

	// Vincular el evento con el usuario que lo creó
	_, linkErr := h.DB.Insert(false, `
		INSERT INTO event_links (user_id, event_id, rol, status) 
		VALUES (?, ?, 'owner', 'approved')`,
		userID, eventID)
	if linkErr != nil {
		fmt.Printf("Error al vincular evento %d con usuario %d: %v\n", eventID, userID, linkErr)
	}

	// Devolver el evento creado
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":         eventID,
		"title":      input.Title,
		"slug":       input.Slug,
		"tags":       input.Tags,
		"content":    input.Content,
		"date_start": input.DateStart,
		"date_end":   input.DateEnd,
		"id_venue":   input.IDVenue,
		"band_ids":   input.BandIDs,
	})
}

// UpdateEvent actualiza un evento existente en la base de datos.
//
// @Summary Actualizar evento
// @Description Actualiza la información de un evento existente
// @Tags eventos
// @Accept json
// @Produce json
// @Param id path int true "ID del evento a actualizar"
// @Param event body Event true "Datos actualizados del evento"
// @Security BearerAuth
// @Success 200 {object} Event "Evento actualizado exitosamente"
// @Failure 400 {string} string "Error en la solicitud"
// @Failure 401 {string} string "No autorizado"
// @Failure 404 {string} string "Evento no encontrado"
// @Failure 500 {string} string "Error interno del servidor"
// @Router /events/{id} [put]
func (h *AuthHandler) UpdateEvent(w http.ResponseWriter, r *http.Request) {
	type EventInput struct {
		IDVenue   int    `json:"id_venue"`
		Title     string `json:"title"`
		Tags      string `json:"tags"`
		Content   string `json:"content"`
		Slug      string `json:"slug"`
		DateStart string `json:"date_start"`
		DateEnd   string `json:"date_end"`
		BandIDs   []int  `json:"band_ids"`
	}

	id := chi.URLParam(r, "id")
	var input EventInput

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "Error al decodificar el cuerpo", http.StatusBadRequest)
		return
	}

	_, err := h.DB.Update(false, `
		UPDATE events SET id_venue=?, title=?, tags=?, content=?, slug=?, date_start=?, date_end=?
		WHERE id = ?`,
		input.IDVenue, input.Title, input.Tags, input.Content, input.Slug, input.DateStart, input.DateEnd, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Eliminar asociaciones anteriores
	_, _ = h.DB.Delete(false, "DELETE FROM events_bands WHERE id_event = ?", id)

	// Insertar nuevas asociaciones
	for _, bandID := range input.BandIDs {
		_, err := h.DB.Insert(false, `
			INSERT INTO events_bands (id_band, id_event) VALUES (?, ?)`,
			bandID, id)
		if err != nil {
			fmt.Printf("Error insertando banda %d: %v\n", bandID, err)
		}
	}

	w.WriteHeader(http.StatusOK)
}

// DeleteEvent elimina un evento de la base de datos.
//
// @Summary Eliminar evento
// @Description Elimina un evento existente y sus relaciones con bandas
// @Tags eventos
// @Produce json
// @Param id path int true "ID del evento a eliminar"
// @Security BearerAuth
// @Success 200 {object} map[string]string "Mensaje de éxito"
// @Failure 400 {string} string "Error en la solicitud"
// @Failure 401 {string} string "No autorizado"
// @Failure 404 {string} string "Evento no encontrado"
// @Failure 500 {string} string "Error interno del servidor"
// @Router /events/{id} [delete]
func (h *AuthHandler) DeleteEvent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// Primero eliminar las relaciones en events_bands
	_, _ = h.DB.Delete(false, "DELETE FROM events_bands WHERE id_event = ?", id)

	// Luego eliminar el evento
	_, err := h.DB.Delete(false, "DELETE FROM events WHERE id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetUserEvents obtiene los eventos vinculados a un usuario específico.
//
// @Summary Obtener eventos de un usuario
// @Description Obtiene los eventos vinculados a un usuario específico
// @Tags eventos
// @Accept json
// @Produce json
// @Param user_id path int true "ID del usuario"
// @Success 200 {array} Event "Lista de eventos vinculados al usuario"
// @Failure 400 {string} string "ID inválido"
// @Failure 500 {string} string "Error al obtener los eventos"
// @Security BearerAuth
// @Router /events/user/{user_id} [get]
func (h *AuthHandler) GetUserEvents(w http.ResponseWriter, r *http.Request) {
	// Configurar encabezados para JSON
	w.Header().Set("Content-Type", "application/json")

	userID := chi.URLParam(r, "user_id")

	// Verificar que el usuario autenticado tenga permiso para ver estos eventos
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
		json.NewEncoder(w).Encode(map[string]string{"error": "No tienes permiso para ver estos eventos"})
		return
	}

	// Consultar los eventos vinculados al usuario
	rows, err := h.DB.Select(`
		SELECT DISTINCT e.id, e.title, e.tags, e.content, e.slug, e.date_start, e.date_end, e.id_venue, v.name, v.address, v.slug, el.rol
		FROM events e
		INNER JOIN event_links el ON e.id = el.event_id
		INNER JOIN venues v ON e.id_venue = v.id
		WHERE el.user_id = ? AND el.status = 'approved'
		ORDER BY e.date_start DESC`, userID)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al consultar los eventos: " + err.Error()})
		return
	}
	defer rows.Close()

	events := []Event{}

	for rows.Next() {
		var event Event
		var venue Venue
		var rol string

		err := rows.Scan(
			&event.ID, &event.Title, &event.Tags, &event.Content, &event.Slug,
			&event.DateStart, &event.DateEnd, &event.VenueID,
			&venue.Name, &venue.Address, &venue.Slug, &rol)

		if err != nil {
			fmt.Printf("Error al escanear evento: %v\n", err)
			continue
		}

		// Asignar el venue al evento
		event.Venue = &venue

		// Agregar el rol como parte de los datos del evento
		event.Rol = rol

		// Obtener las bandas asociadas al evento
		bandRows, err := h.DB.Select(`
			SELECT b.id, b.name, b.slug
			FROM bands b
			JOIN events_bands eb ON b.id = eb.id_band
			WHERE eb.id_event = ?`, event.ID)
		if err == nil {
			var bands []Band
			for bandRows.Next() {
				var band Band
				if err := bandRows.Scan(&band.ID, &band.Name, &band.Slug); err == nil {
					bands = append(bands, band)
				}
			}
			bandRows.Close()
			event.Bands = bands
		}

		events = append(events, event)
	}

	json.NewEncoder(w).Encode(events)
}

// CheckEventSlug verifica si un slug de evento ya existe en la base de datos.
//
// @Summary Verifica disponibilidad de slug
// @Description Comprueba si un slug de evento ya está en uso
// @Tags eventos
// @Accept json
// @Produce json
// @Param slug path string true "Slug a verificar"
// @Success 200 {object} map[string]bool "Slug existe"
// @Failure 400 {string} string "Error: Falta el slug"
// @Failure 404 {string} string "Slug disponible"
// @Failure 500 {string} string "Error al consultar la base de datos"
// @Router /events/slug/{slug} [get]
func (h *AuthHandler) CheckEventSlug(w http.ResponseWriter, r *http.Request) {
	// Configurar encabezados para JSON
	w.Header().Set("Content-Type", "application/json")

	// Obtener el slug de la URL
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Falta el slug"})
		return
	}

	// Verificar si el slug ya existe
	row, err := h.DB.SelectRow("SELECT EXISTS(SELECT 1 FROM events WHERE slug = ?)", slug)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al consultar la base de datos: " + err.Error()})
		return
	}

	var exists bool
	if err := row.Scan(&exists); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al consultar la base de datos: " + err.Error()})
		return
	}

	if exists {
		// El slug ya existe
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]bool{"exists": true})
	} else {
		// El slug está disponible
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]bool{"exists": false})
	}
}
func getSpanishMonth(m time.Month) string {
	months := map[time.Month]string{
		time.January:   "enero",
		time.February:  "febrero",
		time.March:     "marzo",
		time.April:     "abril",
		time.May:       "mayo",
		time.June:      "junio",
		time.July:      "julio",
		time.August:    "agosto",
		time.September: "septiembre",
		time.October:   "octubre",
		time.November:  "noviembre",
		time.December:  "diciembre",
	}
	return months[m]
}

func getSpanishWeekday(w time.Weekday) string {
	days := map[time.Weekday]string{
		time.Sunday:    "Domingo",
		time.Monday:    "Lunes",
		time.Tuesday:   "Martes",
		time.Wednesday: "Miércoles",
		time.Thursday:  "Jueves",
		time.Friday:    "Viernes",
		time.Saturday:  "Sábado",
	}
	return days[w]
}

func (h *AuthHandler) GenerateStoryImageFromEvent(eventID int, flyerURL string) (string, error) {
	event, err := h.getEventByIDInternal(eventID)
	if err != nil {
		return "", fmt.Errorf("no se pudo obtener el evento: %v", err)
	}

	venue, err := h.getVenueByIDInternal(event.VenueID)
	if err != nil {
		return "", fmt.Errorf("no se pudo obtener el venue: %v", err)
	}

	// Formatear fecha y hora
	start, err := time.Parse("2006-01-02 15:04:05", event.DateStart)
	if err != nil {
		return "", fmt.Errorf("error al parsear fecha: %v", err)
	}

	day := fmt.Sprintf("%d", start.Day())
	month := getSpanishMonth(start.Month())
	hour := start.Format("15:04")

	// Ej: "Sábado 13 de abril"
	weekday := getSpanishWeekday(start.Weekday())
	dateStr := fmt.Sprintf("%s %s de %s", weekday, day, month)

	// Ej: "Kalu, Alberdi 90"
	venueStr := venue.Name
	if venue.Address != "" {
		venueStr += ", " + venue.Address
	}

	return GenerateStoryImageFromFlyer(event.Title, flyerURL, dateStr, hour, venueStr)
}
func (h *AuthHandler) PublishEventToInstagramByID(eventID int) error {
	cfg, err := ini.Load("data.conf")
	if err != nil {
		return fmt.Errorf("error al cargar configuración: %v", err)
	}

	accessToken := cfg.Section("instagram").Key("access_token").String()
	businessID := cfg.Section("instagram").Key("business_id").String()
	mediaURL := cfg.Section("spaces").Key("media_url").String()

	// Obtener datos del evento
	var event struct {
		Title     string
		Content   string
		DateStart string
		Slug      string
		VenueName sql.NullString
	}
	row, _ := h.DB.SelectRow(`
		SELECT e.title, e.content, e.date_start, e.slug, v.name as venue_name 
		FROM events e 
		LEFT JOIN venues v ON e.id_venue = v.id 
		WHERE e.id = ?`, eventID)

	if err := row.Scan(&event.Title, &event.Content, &event.DateStart, &event.Slug, &event.VenueName); err != nil {
		return fmt.Errorf("error al obtener datos del evento: %v", err)
	}

	imageURL := fmt.Sprintf("%s/events/%s.jpg", mediaURL, event.Slug)
	venueName := "Lugar a confirmar"
	if event.VenueName.Valid {
		venueName = event.VenueName.String
	}
	contentCleaned := cleanHTML(event.Content)

	caption := fmt.Sprintf("🎵 %s\n\n📍 %s\n📅 %s\n\n%s\n\n#BroteColectivo #AgendaCulturalBroteColectivo #AgendaCultural #Música #Eventos",
		event.Title, venueName, event.DateStart, contentCleaned)

	// Crear media container
	feedURL := fmt.Sprintf("https://graph.facebook.com/v21.0/%s/media?image_url=%s&caption=%s&access_token=%s",
		businessID, url.QueryEscape(imageURL), url.QueryEscape(caption), accessToken)

	feedRes, err := http.Post(feedURL, "application/json", nil)
	if err != nil {
		return fmt.Errorf("error al hacer POST del feed: %v", err)
	}
	defer feedRes.Body.Close()

	var feedData struct {
		ID    string `json:"id"`
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    int    `json:"code"`
		} `json:"error"`
	}
	body, _ := io.ReadAll(feedRes.Body)
	if err := json.Unmarshal(body, &feedData); err != nil {
		return fmt.Errorf("error al decodificar respuesta del feed: %v", err)
	}

	if feedData.ID == "" {
		return fmt.Errorf("error en publicación: %s (Tipo: %s, Código: %d)", feedData.Error.Message, feedData.Error.Type, feedData.Error.Code)
	}

	// Publicar en el feed
	publishURL := fmt.Sprintf("https://graph.facebook.com/v21.0/%s/media_publish?creation_id=%s&access_token=%s",
		businessID, feedData.ID, accessToken)

	publishRes, err := http.Post(publishURL, "application/json", nil)
	if err != nil {
		return fmt.Errorf("error al publicar en feed: %v", err)
	}
	defer publishRes.Body.Close()
	// === GENERAR Y PUBLICAR STORY ===
	generatedPath, err := h.GenerateStoryImageFromEvent(eventID, imageURL)
	if err != nil {
		log.Printf("Error generando imagen de story: %v", err)
		return nil // no detenemos el proceso
	}

	hasher := sha256.New()
	hasher.Write([]byte(imageURL))
	flyerHash := hex.EncodeToString(hasher.Sum(nil))[:8]
	storyObjectPath := fmt.Sprintf("events/stories/%s-%s.jpg", event.Slug, flyerHash)

	err = uploadToSpaces(generatedPath, storyObjectPath, "image/jpeg")
	if err != nil {
		log.Printf("Error subiendo imagen de story: %v", err)
		return nil // no detenemos el proceso
	}

	storyImageURL := fmt.Sprintf("%s/%s", mediaURL, storyObjectPath)

	storyURL := fmt.Sprintf(
		"https://graph.facebook.com/v21.0/%s/media?access_token=%s&media_type=STORIES&image_url=%s",
		businessID,
		accessToken,
		url.QueryEscape(storyImageURL),
	)

	resStory, err := http.Post(storyURL, "application/json", nil)
	if err != nil {
		log.Printf("Error creando story container: %v", err)
		return nil
	}
	defer resStory.Body.Close()

	bodyStory, _ := io.ReadAll(resStory.Body)

	var storyData struct {
		ID    string `json:"id"`
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    int    `json:"code"`
		} `json:"error"`
	}

	if err := json.Unmarshal(bodyStory, &storyData); err != nil {
		log.Printf("Error al parsear story response: %v", err)
		return nil
	}
	if storyData.ID == "" {
		log.Printf("Error al crear story: %s (tipo: %s, código: %d)", storyData.Error.Message, storyData.Error.Type, storyData.Error.Code)
		return nil
	}

	publishStory := fmt.Sprintf("https://graph.facebook.com/v21.0/%s/media_publish?creation_id=%s&access_token=%s",
		businessID, storyData.ID, accessToken,
	)

	_, err = http.Post(publishStory, "application/json", nil)
	if err != nil {
		log.Printf("Error al publicar story: %v", err)
		return nil
	}
	log.Printf("Story publicada correctamente para evento %d", eventID)

	return nil
}

// PublishEventToInstagram publica un evento en Instagram (feed y story)
func (h *AuthHandler) PublishEventToInstagram(w http.ResponseWriter, r *http.Request) {
	fmt.Println("[DEBUG] Iniciando publicación en Instagram")

	// Verificar autenticación (solo admin)
	// claims, ok := r.Context().Value("user").(*models.Claims)
	// if !ok || claims.Role != "admin" {
	// 	http.Error(w, "No autorizado. Se requiere rol de administrador", http.StatusUnauthorized)
	// 	return
	// }

	// Intentar renovar el token antes de publicar
	if err := h.renewInstagramToken(); err != nil {
		fmt.Printf("[DEBUG] Warning: No se pudo renovar el token de Instagram: %v\n", err)
		// Continuamos con el token actual aunque no se haya podido renovar
	}

	// Obtener ID del evento
	eventID := chi.URLParam(r, "id")
	fmt.Printf("[DEBUG] Procesando evento ID: %s\n", eventID)

	// Cargar configuración de Instagram
	cfg, err := ini.Load("data.conf")
	if err != nil {
		fmt.Printf("[DEBUG] Error al cargar configuración: %v\n", err)
		http.Error(w, "Error al cargar configuración", http.StatusInternalServerError)
		return
	}

	accessToken := cfg.Section("instagram").Key("access_token").String()
	businessID := cfg.Section("instagram").Key("business_id").String()
	fmt.Printf("[DEBUG] Business ID: %s\n", businessID)

	// Obtener datos del evento
	var event struct {
		Title     string
		Content   string
		DateStart string
		Slug      string
		VenueName sql.NullString
	}

	row, _ := h.DB.SelectRow(`
		SELECT e.title, e.content, e.date_start, e.slug, v.name as venue_name 
		FROM events e 
		LEFT JOIN venues v ON e.id_venue = v.id 
		WHERE e.id = ?`, eventID)

	if err := row.Scan(&event.Title, &event.Content, &event.DateStart, &event.Slug, &event.VenueName); err != nil {
		fmt.Printf("[DEBUG] Error al obtener datos del evento: %v\n", err)
		http.Error(w, "Error al obtener datos del evento: "+err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Printf("[DEBUG] Datos del evento: Title=%s, Slug=%s, DateStart=%s\n", event.Title, event.Slug, event.DateStart)

	// Preparar la URL de la imagen
	mediaURL := cfg.Section("spaces").Key("media_url").String()
	imageURL := fmt.Sprintf("%s/events/%s.jpg", mediaURL, event.Slug)
	fmt.Printf("[DEBUG] URL de la imagen: %s\n", imageURL)

	// Preparar el texto para Instagram
	venueName := "Lugar a confirmar"
	if event.VenueName.Valid {
		venueName = event.VenueName.String
	}

	// Limpiar el contenido HTML
	contentCleaned := cleanHTML(event.Content)
	fmt.Printf("[DEBUG] Contenido limpio: %s\n", contentCleaned)

	caption := fmt.Sprintf("🎵 %s\n\n📍 %s\n📅 %s\n\n%s\n\n#BroteColectivo #AgendaCulturalBroteColectivo #AgendaCultural #Música #Eventos",
		event.Title,
		venueName,
		event.DateStart,
		contentCleaned,
	)

	// Resultado de la publicación
	result := struct {
		Success bool `json:"success"`
		Feed    struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		} `json:"feed"`
		Story struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		} `json:"story"`
	}{
		Success: true,
	}

	// Publicar en el feed
	feedURL := fmt.Sprintf("https://graph.facebook.com/v21.0/%s/media?image_url=%s&caption=%s&access_token=%s",
		businessID,
		url.QueryEscape(imageURL),
		url.QueryEscape(caption),
		accessToken,
	)
	fmt.Printf("[DEBUG] URL del feed (sin token): %s\n", strings.Replace(feedURL, accessToken, "TOKEN_REMOVED", 1))

	feedRes, err := http.Post(feedURL, "application/json", nil)
	if err != nil {
		fmt.Printf("[DEBUG] Error al hacer POST al feed: %v\n", err)
		result.Feed.Success = false
		result.Feed.Message = err.Error()
		result.Success = false
	} else {
		var feedData struct {
			ID    string `json:"id"`
			Error struct {
				Message        string `json:"message"`
				Type           string `json:"type"`
				Code           int    `json:"code"`
				SubCode        int    `json:"subcode,omitempty"`
				ErrorUserTitle string `json:"error_user_title,omitempty"`
				ErrorUserMsg   string `json:"error_user_msg,omitempty"`
			} `json:"error"`
		}
		body, _ := io.ReadAll(feedRes.Body)
		fmt.Printf("[DEBUG] Respuesta del feed (status %d): %s\n", feedRes.StatusCode, string(body))

		if err := json.Unmarshal(body, &feedData); err != nil {
			fmt.Printf("[DEBUG] Error al decodificar respuesta del feed: %v\n", err)
		}
		feedRes.Body.Close()

		if feedData.ID != "" {
			fmt.Printf("[DEBUG] ID del contenido creado: %s\n", feedData.ID)
			publishURL := fmt.Sprintf("https://graph.facebook.com/v21.0/%s/media_publish?creation_id=%s&access_token=%s",
				businessID,
				feedData.ID,
				accessToken,
			)
			fmt.Printf("[DEBUG] URL de publicación (sin token): %s\n", strings.Replace(publishURL, accessToken, "TOKEN_REMOVED", 1))

			publishRes, err := http.Post(publishURL, "application/json", nil)
			if err != nil {
				fmt.Printf("[DEBUG] Error al publicar contenido: %v\n", err)
				result.Feed.Success = false
				result.Feed.Message = "Error al publicar: " + err.Error()
				result.Success = false
			} else {
				body, _ := io.ReadAll(publishRes.Body)
				fmt.Printf("[DEBUG] Respuesta de publicación (status %d): %s\n", publishRes.StatusCode, string(body))

				var publishData struct {
					ID    string `json:"id"`
					Error struct {
						Message string `json:"message"`
						Type    string `json:"type"`
						Code    int    `json:"code"`
					} `json:"error"`
				}
				if err := json.Unmarshal(body, &publishData); err != nil {
					fmt.Printf("[DEBUG] Error al decodificar respuesta de publicación: %v\n", err)
				} else if publishData.ID != "" {
					fmt.Printf("[DEBUG] Post ID: %s\n", publishData.ID)
					result.Feed.Success = true
					result.Feed.Message = fmt.Sprintf("Publicado correctamente. Post ID: %s", publishData.ID)
				} else if publishData.Error.Message != "" {
					result.Feed.Success = false
					result.Feed.Message = fmt.Sprintf("Error al publicar: %s (Tipo: %s, Código: %d)",
						publishData.Error.Message,
						publishData.Error.Type,
						publishData.Error.Code)
					result.Success = false
				}
				publishRes.Body.Close()
			}
		} else if feedData.Error.Message != "" {
			fmt.Printf("[DEBUG] Error del feed: %+v\n", feedData.Error)
			result.Feed.Success = false
			result.Feed.Message = fmt.Sprintf("Error: %s (Tipo: %s, Código: %d, SubCode: %d)\nTítulo: %s\nMensaje: %s",
				feedData.Error.Message,
				feedData.Error.Type,
				feedData.Error.Code,
				feedData.Error.SubCode,
				feedData.Error.ErrorUserTitle,
				feedData.Error.ErrorUserMsg)
			result.Success = false
		}
	}
	eventIDInt, err := strconv.Atoi(eventID)
	if err != nil {
		log.Printf("Error al convertir eventID a entero: %v", err)
		http.Error(w, "ID del evento inválido", http.StatusBadRequest)
		return
	}

	generatedPath, err := h.GenerateStoryImageFromEvent(eventIDInt, imageURL)
	if err != nil {
		log.Printf("Error generando imagen de story: %v", err)
		// fallback a la imagen original si querés
	}

	hasher := sha256.New()
	hasher.Write([]byte(imageURL))
	flyerHash := hex.EncodeToString(hasher.Sum(nil))[:8] // usamos solo 8 caracteres para no hacerlo tan largo

	storyObjectPath := fmt.Sprintf("events/stories/%s-%s.jpg", event.Slug, flyerHash)

	err = uploadToSpaces(generatedPath, storyObjectPath, "image/jpeg")
	if err != nil {
		log.Printf("Error subiendo imagen de story: %v", err)
		// podés usar imageURL como fallback
	}
	storyImageURL := fmt.Sprintf("%s/%s", mediaURL, storyObjectPath)

	// first create the mediaContainer
	mediaContainerURL := fmt.Sprintf("https://graph.facebook.com/v21.0/%s/media?access_token=%s&media_type=STORIES&image_url=%s",
		businessID,
		accessToken,
		url.QueryEscape(storyImageURL),
	)

	mediaContainerRes, err := http.Post(mediaContainerURL, "application/json", nil)
	if err != nil {
		fmt.Printf("[DEBUG] Error al crear el contenedor de medios: %v\n", err)
		result.Story.Success = false
		result.Story.Message = err.Error()
		result.Success = false
		return
	}
	// Leer el body del response del contenedor de medios
	body, _ := io.ReadAll(mediaContainerRes.Body)

	fmt.Printf("[DEBUG] Respuesta del story (status %d): %s\n", mediaContainerRes.StatusCode, string(body))
	mediaContainerRes.Body.Close()

	// Parsear el JSON para obtener el creation_id
	var storyData struct {
		ID    string `json:"id"`
		Error struct {
			Message        string `json:"message"`
			Type           string `json:"type"`
			Code           int    `json:"code"`
			ErrorUserTitle string `json:"error_user_title,omitempty"`
			ErrorUserMsg   string `json:"error_user_msg,omitempty"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &storyData); err != nil {
		log.Printf("[DEBUG] Error al parsear respuesta del story: %v", err)
		result.Story.Success = false
		result.Story.Message = "Error al parsear respuesta del story"
		result.Success = false
		return
	}

	if storyData.ID == "" {
		log.Printf("[DEBUG] Error del contenedor: %+v\n", storyData.Error)
		result.Story.Success = false
		result.Story.Message = fmt.Sprintf("Error: %s (Tipo: %s, Código: %d)", storyData.Error.Message, storyData.Error.Type, storyData.Error.Code)
		result.Success = false
		return
	}

	// Usar el creation_id correctamente
	creationId := storyData.ID
	storyPublish := fmt.Sprintf("https://graph.facebook.com/v21.0/%s/media_publish?creation_id=%s&access_token=%s",
		businessID,
		creationId,
		accessToken,
	)

	storyPublishRes, err := http.Post(storyPublish, "application/json", nil)
	if err != nil {
		fmt.Printf("[DEBUG] Error al publicar story: %v\n", err)
		result.Story.Success = false
		result.Story.Message = err.Error()
		result.Success = false
		return
	}

	defer storyPublishRes.Body.Close()

	if storyData.ID == "" {
		log.Printf("[DEBUG] Error del story: %+v\n", storyData.Error)
		result.Story.Success = false
		result.Story.Message = fmt.Sprintf("Error: %s (Tipo: %s, Código: %d)", storyData.Error.Message, storyData.Error.Type, storyData.Error.Code)
		result.Success = false
		return
	}

	fmt.Printf("[DEBUG] Story publicado correctamente. Post ID: %s\n", storyData.ID)
	result.Story.Success = true
	result.Story.Message = fmt.Sprintf("Publicado correctamente. Post ID: %s", storyData.ID)
	result.Success = true

	w.Header().Set("Content-Type", "application/json")
	responseJSON, _ := json.MarshalIndent(result, "", "  ")
	fmt.Printf("[DEBUG] Respuesta final: %s\n", string(responseJSON))
	json.NewEncoder(w).Encode(result)
}

// Función auxiliar para formatear la fecha del evento
func formatEventDate(startDate, endDate string) string {
	start, err := time.Parse("2006-01-02T15:04:05Z", startDate)
	if err != nil {
		return startDate
	}

	end, err := time.Parse("2006-01-02T15:04:05Z", endDate)
	if err != nil {
		return fmt.Sprintf("%s", start.Format("02/01/2006 15:04"))
	}

	// Si es el mismo día
	if start.Year() == end.Year() && start.Month() == end.Month() && start.Day() == end.Day() {
		return fmt.Sprintf("%s, %s - %s",
			start.Format("02/01/2006"),
			start.Format("15:04"),
			end.Format("15:04"))
	}

	return fmt.Sprintf("%s - %s",
		start.Format("02/01/2006 15:04"),
		end.Format("02/01/2006 15:04"))
}

// Función auxiliar para truncar texto
func truncateText(text string, maxLength int) string {
	if len(text) <= maxLength {
		return text
	}

	// Truncar en el último espacio antes del límite
	truncated := text[:maxLength]
	lastSpace := strings.LastIndex(truncated, " ")
	if lastSpace > 0 {
		truncated = truncated[:lastSpace]
	}

	return truncated + "..."
}

// GetEventByID obtiene un evento por su ID
func (h *AuthHandler) getEventByIDInternal(id int) (*Event, error) {
	row, err := h.DB.SelectRow(`
		SELECT id, id_venue, title, tags, content, slug, date_start, date_end
		FROM events
		WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}

	var event Event
	err = row.Scan(
		&event.ID,
		&event.VenueID,
		&event.Title,
		&event.Tags,
		&event.Content,
		&event.Slug,
		&event.DateStart,
		&event.DateEnd,
	)
	if err != nil {
		return nil, err
	}

	return &event, nil
}

// GetVenueByID obtiene un venue por su ID
func (h *AuthHandler) getVenueByIDInternal(id int) (*Venue, error) {
	row, err := h.DB.SelectRow(`
		SELECT id, name, address, description, slug, latlng, city
		FROM venues
		WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}

	var venue Venue
	err = row.Scan(
		&venue.ID,
		&venue.Name,
		&venue.Address,
		&venue.Description,
		&venue.Slug,
		&venue.LatLng,
		&venue.City,
	)
	if err != nil {
		return nil, err
	}

	return &venue, nil
}
func GenerateStoryImageFromFlyer(title, flyerURL, date, hour, venue string) (string, error) {
	const (
		width  = 1080
		height = 1920
	)

	// Descargar el flyer
	resp, err := http.Get(flyerURL)
	if err != nil {
		return "", fmt.Errorf("error al descargar el flyer: %v", err)
	}
	defer resp.Body.Close()

	flyerData, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error al leer el flyer: %v", err)
	}

	flyerImg, _, err := image.Decode(bytes.NewReader(flyerData))
	if err != nil {
		return "", fmt.Errorf("error al decodificar el flyer: %v", err)
	}

	bg := image.NewRGBA(image.Rect(0, 0, width, height))

	// Redimensionar el flyer para que ocupe la mitad superior
	flyerBounds := flyerImg.Bounds()
	flyerRatio := float64(flyerBounds.Dx()) / float64(flyerBounds.Dy())
	targetHeight := height / 2
	targetWidth := int(float64(targetHeight) * flyerRatio)
	if targetWidth > width {
		targetWidth = width
		targetHeight = int(float64(width) / flyerRatio)
	}

	flyerResized := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	drawImage.CatmullRom.Scale(flyerResized, flyerResized.Bounds(), flyerImg, flyerImg.Bounds(), drawImage.Over, nil)

	flyerX := (width - targetWidth) / 2
	drawImage.Draw(bg, image.Rect(flyerX, 0, flyerX+targetWidth, targetHeight), flyerResized, image.Point{}, drawImage.Over)

	// Cargar tipografía
	fontBytes, err := os.ReadFile("assets/fonts/Montserrat-Bold.ttf")
	if err != nil {
		return "", err
	}
	f, err := opentype.Parse(fontBytes)
	if err != nil {
		return "", err
	}

	// Caras de fuente
	titleFace, _ := opentype.NewFace(f, &opentype.FaceOptions{Size: 80, DPI: 72, Hinting: font.HintingFull})
	subtitleFace, _ := opentype.NewFace(f, &opentype.FaceOptions{Size: 42, DPI: 72, Hinting: font.HintingFull})
	promoFace, _ := opentype.NewFace(f, &opentype.FaceOptions{Size: 44, DPI: 72, Hinting: font.HintingFull})
	siteFace, _ := opentype.NewFace(f, &opentype.FaceOptions{Size: 40, DPI: 72, Hinting: font.HintingFull})

	d := &font.Drawer{Dst: bg, Src: image.NewUniform(color.White)}

	// Título
	d.Face = titleFace
	maxWidth := width - 100 // Margen de 50px a cada lado

	lines := []string{}
	words := strings.Fields(title)
	line := ""

	for _, word := range words {
		testLine := line + " " + word
		if d.MeasureString(strings.TrimSpace(testLine)).Round() > maxWidth {
			lines = append(lines, strings.TrimSpace(line))
			line = word
		} else {
			line = testLine
		}
	}
	if line != "" {
		lines = append(lines, strings.TrimSpace(line))
	}

	yOffset := targetHeight + 100
	for _, l := range lines {
		lineWidth := d.MeasureString(l).Round()
		d.Dot = fixed.P((width-lineWidth)/2, yOffset)
		d.DrawString(l)
		yOffset += 90 // espacio entre líneas
	}
	// Subtítulo
	subtitle1 := fmt.Sprintf("%s · %s hs", date, hour)
	subtitle2 := venue
	d.Face = subtitleFace
	subtitle1Width := d.MeasureString(subtitle1).Round()
	d.Dot = fixed.P((width-subtitle1Width)/2, yOffset+20)
	d.DrawString(subtitle1)

	subtitle2Width := d.MeasureString(subtitle2).Round()
	d.Dot = fixed.P((width-subtitle2Width)/2, yOffset+70)
	d.DrawString(subtitle2)

	promoText := "Enterate de este evento y otros en #AgendaCultural"
	d.Face = promoFace // ¡Muy importante! Definir la fuente antes de medir

	maxWidthPromo := width - 100
	var linesPromo []string
	wordsPromo := strings.Fields(promoText)
	currentLine := ""

	for _, word := range wordsPromo {
		testLine := strings.TrimSpace(currentLine + " " + word)
		testWidth := font.MeasureString(promoFace, testLine).Round()

		if testWidth > maxWidthPromo && currentLine != "" {
			linesPromo = append(linesPromo, strings.TrimSpace(currentLine))
			currentLine = word
		} else {
			if currentLine != "" {
				currentLine += " "
			}
			currentLine += word
		}
	}
	if currentLine != "" {
		linesPromo = append(linesPromo, currentLine)
	}

	// Dibujar las líneas empezando desde más arriba
	startY := height - 320
	for i, line := range linesPromo {
		lineWidth := d.MeasureString(line).Round()
		d.Dot = fixed.P((width-lineWidth)/2, startY+(i*48))
		d.DrawString(line)
	}

	// Sitio
	siteText := "www.brotecolectivo.com"
	d.Face = siteFace
	siteWidth := d.MeasureString(siteText).Round()
	d.Dot = fixed.P((width-siteWidth)/2, height-200)
	d.DrawString(siteText)

	// Logo
	respLogo, errLogo := http.Get("https://brotecolectivo.com/img/logo.png")
	if errLogo != nil {
		return "", errLogo
	}
	defer respLogo.Body.Close()

	logoData, err := io.ReadAll(respLogo.Body)
	if err != nil {
		return "", err
	}

	logoImg, _, err := image.Decode(bytes.NewReader(logoData))
	if err != nil {
		return "", err
	}

	logoW := logoImg.Bounds().Dx() * 3 / 4 // antes era /2
	logoH := logoImg.Bounds().Dy() * 3 / 4
	logoResized := image.NewRGBA(image.Rect(0, 0, logoW, logoH))
	drawImage.CatmullRom.Scale(logoResized, logoResized.Bounds(), logoImg, logoImg.Bounds(), drawImage.Over, nil)

	destX := (width - logoW) / 2
	destY := height - logoH - 60
	drawImage.Draw(bg, image.Rect(destX, destY, destX+logoW, destY+logoH), logoResized, image.Point{}, drawImage.Over)

	// Guardar imagen
	tmpFile, err := os.CreateTemp("", "story-*.jpg")
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()

	err = jpeg.Encode(tmpFile, bg, &jpeg.Options{Quality: 90})
	if err != nil {
		return "", err
	}

	return tmpFile.Name(), nil
}

// renewInstagramToken intenta renovar el token de acceso de Instagram
func (h *AuthHandler) renewInstagramToken() error {
	cfg, err := ini.Load("data.conf")
	if err != nil {
		return fmt.Errorf("error al cargar configuración: %v", err)
	}

	// Obtener las credenciales
	appID := cfg.Section("instagram").Key("app_id").String()
	appSecret := cfg.Section("instagram").Key("secret_meta").String()
	currentToken := cfg.Section("instagram").Key("access_token").String()

	// URL para renovar el token
	tokenURL := fmt.Sprintf("https://graph.facebook.com/v21.0/oauth/access_token"+
		"?grant_type=fb_exchange_token"+
		"&client_id=%s"+
		"&client_secret=%s"+
		"&fb_exchange_token=%s",
		appID, appSecret, currentToken)

	// Hacer la petición para renovar el token
	resp, err := http.Get(tokenURL)
	if err != nil {
		return fmt.Errorf("error al renovar token: %v", err)
	}
	defer resp.Body.Close()

	// Decodificar la respuesta
	var result struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("error al decodificar respuesta: %v", err)
	}

	if result.AccessToken == "" {
		return fmt.Errorf("no se recibió un nuevo token")
	}

	// Actualizar el token en el archivo de configuración
	cfg.Section("instagram").Key("access_token").SetValue(result.AccessToken)
	if err := cfg.SaveTo("data.conf"); err != nil {
		return fmt.Errorf("error al guardar nuevo token: %v", err)
	}

	return nil
}

// cleanHTML limpia el texto de etiquetas HTML
func cleanHTML(html string) string {
	// Primero reemplazamos algunos elementos comunes con espacios o saltos de línea
	html = strings.ReplaceAll(html, "</p>", "\n\n")
	html = strings.ReplaceAll(html, "</div>", "\n")
	html = strings.ReplaceAll(html, "<br>", "\n\n")
	html = strings.ReplaceAll(html, "<br/>", "\n")
	html = strings.ReplaceAll(html, "<br />", "\n")

	// Removemos todas las etiquetas HTML
	re := regexp.MustCompile("<[^>]*>")
	text := re.ReplaceAllString(html, "")

	// Limpiamos espacios y saltos de línea múltiples
	text = strings.TrimSpace(text)
	re = regexp.MustCompile(`\s+`)
	text = re.ReplaceAllString(text, " ")

	// Decodificamos entidades HTML comunes
	text = strings.ReplaceAll(text, "&nbsp;", " ")
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&quot;", "\"")
	text = strings.ReplaceAll(text, "&#39;", "'")
	// reemplazar los iacute, ntilde y demas
	text = strings.ReplaceAll(text, "&iacute;", "í")
	text = strings.ReplaceAll(text, "&ntilde;", "ñ")
	text = strings.ReplaceAll(text, "&aacute;", "á")
	text = strings.ReplaceAll(text, "&eacute;", "é")

	text = strings.ReplaceAll(text, "&oacute;", "ó")
	text = strings.ReplaceAll(text, "&uacute;", "ú")
	// los acutes en mayuscula
	text = strings.ReplaceAll(text, "&Aacute;", "Á")
	text = strings.ReplaceAll(text, "&Eacute;", "É")
	text = strings.ReplaceAll(text, "&Iacute;", "Í")
	text = strings.ReplaceAll(text, "&Oacute;", "Ó")
	text = strings.ReplaceAll(text, "&Uacute;", "Ú")
	text = strings.ReplaceAll(text, "&Ntilde;", "Ñ")

	text = strings.ReplaceAll(text, "&iexcl;", "¡")
	text = strings.ReplaceAll(text, "&iquest;", "¿")
	text = strings.ReplaceAll(text, "&copy;", "©")
	text = strings.ReplaceAll(text, "&reg;", "®")
	text = strings.ReplaceAll(text, "&trade;", "™")

	return text
}

// GenerateEventDescription genera una descripción para un evento usando OpenAI
func (h *AuthHandler) GenerateEventDescription(w http.ResponseWriter, r *http.Request) {
	type GenerateDescriptionRequest struct {
		Title        string   `json:"title"`
		VenueName    string   `json:"venue_name"`
		VenueAddress string   `json:"venue_address"`
		Date         string   `json:"date"`
		Bands        []string `json:"bands"`
		CustomPrompt string   `json:"custom_prompt"`
	}

	type GenerateDescriptionResponse struct {
		Description string `json:"description"`
	}

	var requestData GenerateDescriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&requestData); err != nil {
		http.Error(w, "Error al procesar la solicitud", http.StatusBadRequest)
		return
	}

	prompt := fmt.Sprintf(`Genera una descripción atractiva para un evento musical con la siguiente información:
Título: %s
Lugar: %s
Dirección: %s
Fecha: %s
Bandas: %s
%s

Instrucciones:
1. Escribe en español de Argentina
2. Usa un tono profesional pero amigable
3. Incluye toda la información proporcionada de manera natural, NO inventes detalles
4. Estructura: Comienza con la fecha y lugar, luego describe el evento y las bandas, y finaliza con detalles prácticos
5. Si se proporciona información adicional, agrégala solo si es factual y relevante
6. No uses puntos y aparte, solo punto final
7. No uses emojis.
8. Usa formato HTML para resaltar elementos importantes (<b> para nombres, <i> para géneros)
9. Separa bien los párrafos con punto y aparte, los parrafos no pueden tener mas de 2 oraciones, usa <p> para separar los parrafos.
10. Si bien, no te digo de usar modismos argentinos, pero evitar usar Ven o palabras asi, usa veni, y recordá que el sitio es un portal cultural.


Por ejemplo: El sábado 19 de abril a las 20 hs, el escenario de Ensayo Abierto recibe a Fermantic, banda invitada directamente desde Punta Arenas (Chile). Con un sonido que cruza el rock alternativo con influencias del sur patagónico, el trío se presenta por primera vez en Río Gallegos. La fecha será en el espacio cultural ubicado en Zapiola 353, y forma parte del ciclo de shows organizados por Sonoman, que sigue fortaleciendo el vínculo musical entre ambos lados de la cordillera. Una noche ideal para descubrir nuevos sonidos y compartir una experiencia distinta, en un ambiente íntimo y con entrada libre hasta completar la capacidad del lugar.

Información proporcionada: %s`,
		requestData.Title,
		requestData.VenueName,
		requestData.VenueAddress,
		requestData.Date,
		strings.Join(requestData.Bands, ", "),
		requestData.CustomPrompt)

	openaiURL := "https://api.openai.com/v1/chat/completions"
	requestBody := map[string]interface{}{
		"model": "gpt-3.5-turbo",
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": prompt,
			},
		},
		"temperature": 0.5,
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		http.Error(w, "Error al preparar la solicitud", http.StatusInternalServerError)
		return
	}

	req, err := http.NewRequest("POST", openaiURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		http.Error(w, "Error al crear la solicitud", http.StatusInternalServerError)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+getOpenAIKey())

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "Error al realizar la solicitud a OpenAI", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	var openaiResponse struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&openaiResponse); err != nil {
		http.Error(w, "Error al leer la respuesta de OpenAI", http.StatusInternalServerError)
		return
	}

	if len(openaiResponse.Choices) == 0 {
		http.Error(w, "No se recibió respuesta de OpenAI", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(GenerateDescriptionResponse{
		Description: openaiResponse.Choices[0].Message.Content,
	})
}
