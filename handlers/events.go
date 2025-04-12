package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"brotecolectivo/models"

	"github.com/go-chi/chi/v5"
	"gopkg.in/ini.v1"
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
		row, err = h.DB.SelectRow(`

		SELECT

			e.id, e.title, e.tags, e.content, e.slug, e.date_start, e.date_end,
			v.id, v.name, v.latlng, v.address, v.city
		FROM events e
		JOIN venues v ON e.id_venue = v.id
		WHERE e.id = ?
	`, id)
	} else {
		// No es número → buscar por slug
		row, err = h.DB.SelectRow(`
		SELECT
			e.id, e.title, e.tags, e.content, e.slug, e.date_start, e.date_end,
			v.id, v.name, v.latlng, v.address, v.city
		FROM events e
		JOIN venues v ON e.id_venue = v.id
		WHERE e.slug = ?
	`, id)
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
	row, err := h.DB.SelectRow("SELECT EXISTS(SELECT 1 FROM events WHERE id = ?)", eventID)
	if err != nil {
		http.Error(w, "Error al verificar evento: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := row.Scan(&exists); err != nil {
		http.Error(w, "Error al escanear resultado: "+err.Error(), http.StatusInternalServerError)
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
		SELECT e.id, e.title, e.tags, e.content, e.slug, e.date_start, e.date_end, e.id_venue, v.name, v.address, v.slug, el.rol
		FROM events e
		JOIN event_links el ON e.id = el.event_id
		JOIN venues v ON e.id_venue = v.id
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

		if err := rows.Scan(
			&event.ID, &event.Title, &event.Tags, &event.Content, &event.Slug,
			&event.DateStart, &event.DateEnd, &event.VenueID,
			&venue.Name, &venue.Address, &venue.Slug, &rol); err != nil {
			continue // Saltamos este registro si hay error
		}

		// Asignar el venue al evento
		event.Venue = &venue

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
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al leer el resultado: " + err.Error()})
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

// PublishEventToInstagram publica un evento en Instagram (feed y story)
// @Summary Publicar evento en Instagram
// @Description Publica un evento en Instagram como post y story
// @Tags eventos
// @Accept json
// @Produce json
// @Param id path int true "ID del evento"
// @Security BearerAuth
// @Success 200 {object} map[string]interface{} "Resultado de la publicación"
// @Failure 400 {string} string "Error en la solicitud"
// @Failure 401 {string} string "No autorizado"
// @Failure 500 {string} string "Error interno del servidor"
// @Router /events/{id}/publish-instagram [post]
func (h *AuthHandler) PublishEventToInstagram(w http.ResponseWriter, r *http.Request) {
	// Verificar autenticación (solo admin)
	claims, ok := r.Context().Value("user").(*models.Claims)
	if !ok || claims.Role != "admin" {
		http.Error(w, "No autorizado. Se requiere rol de administrador", http.StatusUnauthorized)
		return
	}

	// Obtener ID del evento
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "ID inválido", http.StatusBadRequest)
		return
	}

	// Obtener datos del evento
	event, err := h.getEventByIDInternal(id)
	if err != nil {
		http.Error(w, "Evento no encontrado", http.StatusNotFound)
		return
	}

	// Obtener datos del venue
	venue, err := h.getVenueByIDInternal(event.VenueID)
	if err != nil {
		http.Error(w, "Venue no encontrado", http.StatusNotFound)
		return
	}

	// Cargar configuración
	cfg, err := ini.Load("data.conf")
	if err != nil {
		http.Error(w, "Error al cargar configuración", http.StatusInternalServerError)
		return
	}

	// Obtener token de Instagram
	instagramToken := cfg.Section("instagram").Key("access_token").String()
	if instagramToken == "" {
		http.Error(w, "Token de Instagram no configurado", http.StatusInternalServerError)
		return
	}

	// Obtener ID de Instagram Business
	instagramBusinessID := cfg.Section("instagram").Key("business_id").String()
	if instagramBusinessID == "" {
		http.Error(w, "ID de Instagram Business no configurado", http.StatusInternalServerError)
		return
	}

	// Preparar el resultado
	result := map[string]interface{}{
		"success": false,
		"feed": map[string]interface{}{
			"success": false,
			"message": "",
		},
		"story": map[string]interface{}{
			"success": false,
			"message": "",
		},
	}

	// Publicar en feed
	feedResult, feedErr := h.publishToInstagramFeed(event, venue, instagramToken, instagramBusinessID)
	if feedErr == nil {
		result["feed"] = map[string]interface{}{
			"success": true,
			"message": "Publicado exitosamente en el feed",
			"post_id": feedResult,
		}
		result["success"] = true
	} else {
		result["feed"] = map[string]interface{}{
			"success": false,
			"message": feedErr.Error(),
		}
	}

	// Publicar en story
	storyResult, storyErr := h.publishToInstagramStory(event, venue, instagramToken, instagramBusinessID)
	if storyErr == nil {
		result["story"] = map[string]interface{}{
			"success":  true,
			"message":  "Publicado exitosamente en stories",
			"story_id": storyResult,
		}
		result["success"] = true
	} else {
		result["story"] = map[string]interface{}{
			"success": false,
			"message": storyErr.Error(),
		}
	}

	// Devolver resultado
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// publishToInstagramFeed publica un evento en el feed de Instagram
func (h *AuthHandler) publishToInstagramFeed(event *Event, venue *Venue, token, businessID string) (string, error) {
	// Ruta de la imagen del evento
	imagePath := fmt.Sprintf("events/%s.jpg", event.Slug)

	// Verificar si la imagen existe en Spaces
	spacesBucket := os.Getenv("SPACES_BUCKET")
	if spacesBucket == "" {
		spacesBucket = "brotecolectivo"
	}

	// Preparar la URL de la imagen
	imageURL := fmt.Sprintf("https://%s.nyc3.digitaloceanspaces.com/%s", spacesBucket, imagePath)

	// Preparar el texto de la publicación
	caption := fmt.Sprintf("🎵 %s\n\n📍 %s\n🗓️ %s\n\n%s\n\n#brotecolectivo #música #eventos #cultura",
		event.Title,
		venue.Name,
		formatEventDate(event.DateStart, event.DateEnd),
		truncateText(event.Content, 200))

	// Primero, crear un container
	containerURL := fmt.Sprintf("https://graph.facebook.com/v17.0/%s/media", businessID)
	containerData := map[string]string{
		"image_url": imageURL,
		"caption":   caption,
	}
	containerJSON, _ := json.Marshal(containerData)

	containerReq, _ := http.NewRequest("POST", containerURL, bytes.NewBuffer(containerJSON))
	containerReq.Header.Set("Content-Type", "application/json")
	q := containerReq.URL.Query()
	q.Add("access_token", token)
	containerReq.URL.RawQuery = q.Encode()

	client := &http.Client{}
	containerResp, err := client.Do(containerReq)
	if err != nil {
		return "", fmt.Errorf("error al crear container: %v", err)
	}
	defer containerResp.Body.Close()

	if containerResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(containerResp.Body)
		return "", fmt.Errorf("error de API (container): %s", string(body))
	}

	var containerResult struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(containerResp.Body).Decode(&containerResult); err != nil {
		return "", fmt.Errorf("error al decodificar respuesta: %v", err)
	}

	// Publicar el container
	publishURL := fmt.Sprintf("https://graph.facebook.com/v17.0/%s/media_publish", businessID)
	publishData := map[string]string{
		"creation_id": containerResult.ID,
	}
	publishJSON, _ := json.Marshal(publishData)

	publishReq, _ := http.NewRequest("POST", publishURL, bytes.NewBuffer(publishJSON))
	publishReq.Header.Set("Content-Type", "application/json")
	q = publishReq.URL.Query()
	q.Add("access_token", token)
	publishReq.URL.RawQuery = q.Encode()

	publishResp, err := client.Do(publishReq)
	if err != nil {
		return "", fmt.Errorf("error al publicar: %v", err)
	}
	defer publishResp.Body.Close()

	if publishResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(publishResp.Body)
		return "", fmt.Errorf("error de API (publish): %s", string(body))
	}

	var publishResult struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(publishResp.Body).Decode(&publishResult); err != nil {
		return "", fmt.Errorf("error al decodificar respuesta: %v", err)
	}

	return publishResult.ID, nil
}

// publishToInstagramStory publica un evento en las stories de Instagram
func (h *AuthHandler) publishToInstagramStory(event *Event, venue *Venue, token, businessID string) (string, error) {
	// Ruta de la imagen del evento
	imagePath := fmt.Sprintf("events/%s.jpg", event.Slug)

	// Verificar si la imagen existe en Spaces
	spacesBucket := os.Getenv("SPACES_BUCKET")
	if spacesBucket == "" {
		spacesBucket = "brotecolectivo"
	}

	// Preparar la URL de la imagen
	imageURL := fmt.Sprintf("https://%s.nyc3.digitaloceanspaces.com/%s", spacesBucket, imagePath)

	// Para stories, primero necesitamos generar una imagen adaptada para stories (9:16)
	// Esto requeriría un servicio de procesamiento de imágenes
	// Por ahora, usaremos la imagen original

	// Crear un container para la story
	containerURL := fmt.Sprintf("https://graph.facebook.com/v17.0/%s/media", businessID)
	containerData := map[string]string{
		"image_url":  imageURL,
		"media_type": "STORIES",
	}
	containerJSON, _ := json.Marshal(containerData)

	containerReq, _ := http.NewRequest("POST", containerURL, bytes.NewBuffer(containerJSON))
	containerReq.Header.Set("Content-Type", "application/json")
	q := containerReq.URL.Query()
	q.Add("access_token", token)
	containerReq.URL.RawQuery = q.Encode()

	client := &http.Client{}
	containerResp, err := client.Do(containerReq)
	if err != nil {
		return "", fmt.Errorf("error al crear container para story: %v", err)
	}
	defer containerResp.Body.Close()

	if containerResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(containerResp.Body)
		return "", fmt.Errorf("error de API (container story): %s", string(body))
	}

	var containerResult struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(containerResp.Body).Decode(&containerResult); err != nil {
		return "", fmt.Errorf("error al decodificar respuesta: %v", err)
	}

	// Publicar el container como story
	publishURL := fmt.Sprintf("https://graph.facebook.com/v17.0/%s/media_publish", businessID)
	publishData := map[string]string{
		"creation_id": containerResult.ID,
	}
	publishJSON, _ := json.Marshal(publishData)

	publishReq, _ := http.NewRequest("POST", publishURL, bytes.NewBuffer(publishJSON))
	publishReq.Header.Set("Content-Type", "application/json")
	q = publishReq.URL.Query()
	q.Add("access_token", token)
	publishReq.URL.RawQuery = q.Encode()

	publishResp, err := client.Do(publishReq)
	if err != nil {
		return "", fmt.Errorf("error al publicar story: %v", err)
	}
	defer publishResp.Body.Close()

	if publishResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(publishResp.Body)
		return "", fmt.Errorf("error de API (publish story): %s", string(body))
	}

	var publishResult struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(publishResp.Body).Decode(&publishResult); err != nil {
		return "", fmt.Errorf("error al decodificar respuesta: %v", err)
	}

	return publishResult.ID, nil
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
