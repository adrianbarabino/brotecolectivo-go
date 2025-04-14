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
	"bytes"

	"brotecolectivo/models"

	"github.com/go-chi/chi/v5"
	"gopkg.in/ini.v1"
)

// Band representa la estructura de datos de un artista o banda musical en el sistema.
// Se utiliza tanto para almacenar en la base de datos como para la respuesta JSON de la API.
type Band struct {
	ID     int               `json:"id"`     // Identificador único de la banda
	Name   string            `json:"name"`   // Nombre de la banda o artista
	Bio    string            `json:"bio"`    // Biografía o descripción del artista
	Slug   string            `json:"slug"`   // Identificador URL-friendly para rutas amigables
	Social map[string]string `json:"social"` // Mapa de redes sociales (clave: plataforma, valor: enlace)
}

// getBucket obtiene el nombre del bucket de almacenamiento desde el archivo de configuración.
// Utilizado para operaciones de almacenamiento de archivos en DigitalOcean Spaces.
func getBucket() string {
	cfg, _ := ini.Load("data.conf")
	return cfg.Section("spaces").Key("bucket").String()
}

// getEndpoint obtiene la URL del endpoint de almacenamiento desde el archivo de configuración.
// Utilizado para operaciones de almacenamiento de archivos en DigitalOcean Spaces.
func getEndpoint() string {
	cfg, _ := ini.Load("data.conf")
	return cfg.Section("spaces").Key("endpoint").String()
}

// getOpenAIKey obtiene la clave de API de OpenAI desde el archivo de configuración
func getOpenAIKey() string {
	cfg, _ := ini.Load("data.conf")
	return cfg.Section("openai").Key("api_key").String()
}

// CheckBandSlug verifica si un slug de banda ya existe en la base de datos.
//
// @Summary Verifica disponibilidad de slug
// @Description Comprueba si un slug de banda ya está en uso
// @Tags bands
// @Accept json
// @Produce json
// @Param slug path string true "Slug a verificar"
// @Success 200 {object} map[string]bool "Slug existe"
// @Failure 400 {string} string "Error: Falta el slug"
// @Failure 404 {string} string "Slug disponible"
// @Failure 500 {string} string "Error al consultar la base de datos"
// @Router /bands/slug/{slug} [get]
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

// GetBandsCount devuelve el número total de bandas/artistas en la base de datos.
//
// @Summary Obtiene el conteo total de bandas
// @Description Devuelve el número total de bandas/artistas registrados
// @Tags bands
// @Accept json
// @Produce json
// @Success 200 {object} map[string]int "Conteo de bandas"
// @Failure 500 {string} string "Error al contar artistas o leer el conteo"
// @Router /bands/count [get]
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

// UploadBandImage sube y procesa una imagen para una banda/artista.
// La imagen se redimensiona y se almacena en DigitalOcean Spaces.
//
// @Summary Sube imagen de banda
// @Description Sube y procesa una imagen para un artista o banda
// @Tags bands
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "Archivo de imagen a subir"
// @Param slug formData string true "Slug de la banda"
// @Success 200 {object} map[string]string "URL de la imagen subida"
// @Failure 400 {string} string "Error en los parámetros o formato de imagen"
// @Failure 500 {string} string "Error al procesar o guardar la imagen"
// @Security BearerAuth
// @Router /bands/upload-image [post]
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

// GetBandsDatatable obtiene datos de bandas formateados para DataTables.
// Soporta paginación, búsqueda y ordenamiento.
//
// @Summary Obtiene bandas para DataTables
// @Description Devuelve datos de bandas formateados para DataTables con soporte para paginación, búsqueda y ordenamiento
// @Tags bands
// @Accept json
// @Produce json
// @Param draw query int false "Parámetro draw de DataTables"
// @Param start query int false "Índice de inicio para paginación"
// @Param length query int false "Número de registros a devolver"
// @Param search[value] query string false "Término de búsqueda"
// @Param order[0][column] query int false "Índice de columna para ordenar"
// @Param order[0][dir] query string false "Dirección de ordenamiento (asc/desc)"
// @Success 200 {object} map[string]interface{} "Datos para DataTables"
// @Failure 500 {string} string "Error al consultar datos"
// @Router /bands/table [get]
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

// GetBands obtiene una lista de bandas/artistas con soporte para filtrado y paginación.
//
// @Summary Lista de bandas
// @Description Obtiene una lista de bandas/artistas con soporte para filtrado y paginación
// @Tags bands
// @Accept json
// @Produce json
// @Param limit query int false "Límite de resultados (por defecto 20)"
// @Param offset query int false "Desplazamiento para paginación"
// @Param search query string false "Término de búsqueda"
// @Success 200 {array} Band "Lista de bandas"
// @Failure 500 {string} string "Error al consultar bandas"
// @Router /bands [get]
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
	} else {
		// Por defecto, ordenar por ID descendente (del más nuevo al más viejo)
		query += " ORDER BY id DESC"
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

// GetBandByID obtiene los detalles de una banda/artista específico por su ID.
//
// @Summary Detalle de banda
// @Description Obtiene los detalles completos de una banda/artista por su ID
// @Tags bands
// @Accept json
// @Produce json
// @Param id path int true "ID de la banda"
// @Success 200 {object} Band "Detalles de la banda"
// @Failure 400 {string} string "ID inválido"
// @Failure 404 {string} string "Banda no encontrada"
// @Failure 500 {string} string "Error al consultar la banda"
// @Router /bands/{id} [get]
func (h *AuthHandler) GetBandByID(w http.ResponseWriter, r *http.Request) {
	// Configurar encabezados para JSON
	w.Header().Set("Content-Type", "application/json")

	id := chi.URLParam(r, "id")
	var b Band
	var socialRaw []byte

	// allow to id could be a slug
	if _, err := strconv.Atoi(id); err != nil {
		// es un slug, buscamos por slug
		row, err := h.DB.SelectRow("SELECT id, name, bio, slug, social FROM bands WHERE slug = ?", id)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al consultar la base de datos: " + err.Error()})
			return
		}
		err = row.Scan(&b.ID, &b.Name, &b.Bio, &b.Slug, &socialRaw)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Artista no encontrado con slug: " + id})
			return
		}
	} else {
		row, err := h.DB.SelectRow("SELECT id, name, bio, slug, social FROM bands WHERE id = ?", id)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al consultar la base de datos: " + err.Error()})
			return
		}

		err = row.Scan(&b.ID, &b.Name, &b.Bio, &b.Slug, &socialRaw)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Artista no encontrado con ID: " + id})
			return
		}
	}

	// Inicializar Social como un mapa vacío si es nil
	b.Social = map[string]string{}

	if socialRaw != nil && len(socialRaw) > 0 {
		if err := json.Unmarshal(socialRaw, &b.Social); err != nil {
			// Si hay error al deserializar, mantenemos el mapa vacío
			fmt.Printf("Error al deserializar social para banda %d: %v\n", b.ID, err)
		}
	}

	// Asegurarnos de que todos los campos estén inicializados
	if b.Name == "" {
		b.Name = "Sin nombre"
	}
	if b.Bio == "" {
		b.Bio = ""
	}
	if b.Slug == "" {
		b.Slug = fmt.Sprintf("band-%d", b.ID)
	}

	json.NewEncoder(w).Encode(b)
}

// CreateBand crea una nueva banda/artista en la base de datos.
//
// @Summary Crear banda
// @Description Crea un nuevo registro de banda/artista
// @Tags bands
// @Accept json
// @Produce json
// @Param band body Band true "Datos de la banda a crear"
// @Success 201 {object} Band "Banda creada"
// @Failure 400 {string} string "Datos inválidos"
// @Failure 500 {string} string "Error al crear la banda"
// @Security BearerAuth
// @Router /bands [post]
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

// UpdateBand actualiza los datos de una banda/artista existente.
//
// @Summary Actualizar banda
// @Description Actualiza los datos de una banda/artista existente
// @Tags bands
// @Accept json
// @Produce json
// @Param id path int true "ID de la banda a actualizar"
// @Param band body Band true "Nuevos datos de la banda"
// @Success 200 {object} Band "Banda actualizada"
// @Failure 400 {string} string "ID o datos inválidos"
// @Failure 404 {string} string "Banda no encontrada"
// @Failure 500 {string} string "Error al actualizar la banda"
// @Security BearerAuth
// @Router /bands/{id} [put]
func (h *AuthHandler) UpdateBand(w http.ResponseWriter, r *http.Request) {
	// Configurar encabezados para JSON
	w.Header().Set("Content-Type", "application/json")

	var b Band
	id := chi.URLParam(r, "id")

	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al decodificar el cuerpo: " + err.Error()})
		return
	}

	socialJSON, _ := json.Marshal(b.Social)

	_, err := h.DB.Update(false, "UPDATE bands SET name=?, bio=?, slug=?, social=? WHERE id=?", b.Name, b.Bio, b.Slug, string(socialJSON), id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al actualizar el artista: " + err.Error()})
		return
	}

	// Obtener los datos actualizados para devolverlos en la respuesta
	var updatedBand Band
	var socialRaw []byte

	row, err := h.DB.SelectRow("SELECT id, name, bio, slug, social FROM bands WHERE id = ?", id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Artista actualizado pero no se pudo recuperar la información actualizada"})
		return
	}

	err = row.Scan(&updatedBand.ID, &updatedBand.Name, &updatedBand.Bio, &updatedBand.Slug, &socialRaw)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Artista actualizado pero no se pudo recuperar la información actualizada"})
		return
	}

	// Inicializar Social como un mapa vacío si es nil
	updatedBand.Social = map[string]string{}

	if socialRaw != nil && len(socialRaw) > 0 {
		if err := json.Unmarshal(socialRaw, &updatedBand.Social); err != nil {
			// Si hay error al deserializar, mantenemos el mapa vacío
			fmt.Printf("Error al deserializar social para banda %d: %v\n", updatedBand.ID, err)
		}
	}

	// Devolver los datos actualizados
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(updatedBand)
}

// DeleteBand elimina una banda/artista de la base de datos.
func (h *AuthHandler) DeleteBand(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// Primero eliminar las referencias en artist_links
	_, err := h.DB.Delete(false, "DELETE FROM artist_links WHERE artist_id = ?", id)
	if err != nil {
		http.Error(w, "Error al eliminar referencias de artist_links: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Eliminar las referencias en events_bands si existen
	_, err = h.DB.Delete(false, "DELETE FROM events_bands WHERE id_band = ?", id)
	if err != nil {
		http.Error(w, "Error al eliminar referencias de events_bands: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Finalmente eliminar la banda
	result, err := h.DB.Delete(false, "DELETE FROM bands WHERE id = ?", id)
	if err != nil {
		http.Error(w, "Error al eliminar la banda: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if result == 0 {
		http.Error(w, "No se encontró la banda", http.StatusNotFound)
		return
	}

	// Respuesta exitosa
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
		"message": "Banda eliminada correctamente",
	})
}

// GetUserBands obtiene los artistas vinculados a un usuario específico.
//
// @Summary Obtener artistas de un usuario
// @Description Obtiene los artistas vinculados a un usuario específico
// @Tags bands
// @Accept json
// @Produce json
// @Param user_id path int true "ID del usuario"
// @Success 200 {array} Band "Lista de artistas vinculados al usuario"
// @Failure 400 {string} string "ID inválido"
// @Failure 500 {string} string "Error al obtener los artistas"
// @Security BearerAuth
// @Router /bands/user/{user_id} [get]
func (h *AuthHandler) GetUserBands(w http.ResponseWriter, r *http.Request) {
	// Configurar encabezados para JSON
	w.Header().Set("Content-Type", "application/json")

	userID := chi.URLParam(r, "user_id")

	// Verificar que el usuario autenticado tenga permiso para ver estos artistas
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
		json.NewEncoder(w).Encode(map[string]string{"error": "No tienes permiso para ver estos artistas"})
		return
	}

	// Consultar los artistas vinculados al usuario
	rows, err := h.DB.Select(`
		SELECT DISTINCT b.id, b.name, b.bio, b.slug, b.social, al.rol
		FROM bands b
		INNER JOIN artist_links al ON b.id = al.artist_id
		WHERE al.user_id = ? AND al.status = 'approved'
		ORDER BY b.name ASC`, userID)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al consultar los artistas: " + err.Error()})
		return
	}
	defer rows.Close()

	bands := []Band{}

	for rows.Next() {
		var band Band
		var socialRaw []byte
		var rol string

		if err := rows.Scan(&band.ID, &band.Name, &band.Bio, &band.Slug, &socialRaw, &rol); err != nil {
			fmt.Printf("Error al escanear banda: %v\n", err)
			continue // Saltamos este registro si hay error
		}

		// Inicializar Social como un mapa vacío si es nil
		band.Social = map[string]string{}

		if socialRaw != nil && len(socialRaw) > 0 {
			if err := json.Unmarshal(socialRaw, &band.Social); err != nil {
				// Si hay error al deserializar, mantenemos el mapa vacío
				fmt.Printf("Error al deserializar social para banda %d: %v\n", band.ID, err)
			}
		}

		// Agregar el rol a los datos sociales para mostrarlo en el frontend
		band.Social["rol"] = rol

		bands = append(bands, band)
	}

	json.NewEncoder(w).Encode(bands)
}

// SearchBands busca artistas por nombre
func (h *AuthHandler) SearchBands(w http.ResponseWriter, r *http.Request) {
	// Establecer encabezados CORS
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	// Manejar solicitudes OPTIONS
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Obtener el término de búsqueda
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "Parámetro de búsqueda 'q' requerido", http.StatusBadRequest)
		return
	}

	// Buscar artistas que coincidan con el nombre
	rows, err := h.DB.Select(
		`SELECT id, name, slug FROM bands 
		WHERE name LIKE ? 
		ORDER BY name ASC LIMIT 10`,
		"%"+query+"%",
	)
	if err != nil {
		http.Error(w, "Error al buscar artistas: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	// Construir la respuesta
	var bands []map[string]interface{}
	for rows.Next() {
		var id int
		var name, slug string
		if err := rows.Scan(&id, &name, &slug); err != nil {
			continue
		}
		bands = append(bands, map[string]interface{}{
			"id":   id,
			"name": name,
			"slug": slug,
		})
	}

	// Devolver los resultados como JSON
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(bands)
}

// GenerateArtistBio genera una biografía para un artista usando OpenAI
func (h *AuthHandler) GenerateArtistBio(w http.ResponseWriter, r *http.Request) {
	// Obtener el prompt del cuerpo de la solicitud
	var requestData struct {
		Name string `json:"name"`
		Title string `json:"title"`
		CustomPrompt string `json:"custom_prompt,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&requestData); err != nil {
		http.Error(w, "Error al leer el cuerpo de la solicitud", http.StatusBadRequest)
		return
	}

	// Construir el prompt base
	basePrompt := fmt.Sprintf(`Genera una descripción breve y concisa para la banda %s. 
Reglas:
1. Usa solo la información proporcionada, NO inventes detalles
2. Usa formato HTML para resaltar elementos importantes (<b> para nombres, <i> para géneros)
3. Máximo 2 oraciones cortas
4. Estructura: [Nombre banda] es una banda de [género] integrada por [miembros con sus instrumentos]
5. Si se proporciona información adicional, agrégala solo si es factual y relevante
6. No uses puntos y aparte, solo punto final

Información proporcionada: %s`, requestData.Name, requestData.CustomPrompt)
	
	// Combinar con el prompt personalizado si existe
	finalPrompt := basePrompt

	// Preparar la solicitud a OpenAI
	openaiURL := "https://api.openai.com/v1/chat/completions"
	requestBody := map[string]interface{}{
		"model": "gpt-3.5-turbo",
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": finalPrompt,
			},
		},
		"temperature": 0.5, // Reducir la creatividad para obtener respuestas más concisas y factuales
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		http.Error(w, "Error al preparar la solicitud", http.StatusInternalServerError)
		return
	}

	// Crear la solicitud HTTP
	req, err := http.NewRequest("POST", openaiURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		http.Error(w, "Error al crear la solicitud", http.StatusInternalServerError)
		return
	}

	// Agregar headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+getOpenAIKey())

	// Realizar la solicitud
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "Error al realizar la solicitud a OpenAI", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	// Leer la respuesta
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

	// Verificar que hay una respuesta
	if len(openaiResponse.Choices) == 0 {
		http.Error(w, "No se recibió respuesta de OpenAI", http.StatusInternalServerError)
		return
	}

	// Devolver la biografía generada
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"bio": openaiResponse.Choices[0].Message.Content,
	})
}
