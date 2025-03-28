package handlers

import (
	"brotecolectivo/models"
	"brotecolectivo/utils"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

type PublicUser struct {
	ID        int       `json:"id"`
	Username  string    `json:"username"`
	Role      string    `json:"role"`
	Email     string    `json:"email"`
	Name      string    `json:"realName"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Provider  string    `json:"provider"`
}

func normalizeUTF8(input string) string {
	t := transform.Chain(norm.NFD, transform.RemoveFunc(isMn), norm.NFC)
	result, _, err := transform.String(t, input)
	if err != nil {
		return input
	}
	return result
}

func isMn(r rune) bool {
	return norm.NFD.QuickSpan([]byte(string(r))) == len([]byte(string(r))) && !utf8.ValidRune(r)
}
func (h *AuthHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Name     string `json:"realName"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "Datos inválidos", http.StatusBadRequest)
		return
	}

	// Validación básica
	if input.Username == "" || input.Email == "" || input.Password == "" {
		http.Error(w, "Faltan campos requeridos", http.StatusBadRequest)
		return
	}

	// Verificar si el usuario ya existe
	if utils.UsernameExists(input.Username, h.DB) {
		http.Error(w, "El usuario ya existe", http.StatusConflict)
		return
	}

	salt := utils.GenerateSalt()
	hashedPassword := utils.HashPassword(input.Password, salt)

	id, err := h.DB.Insert(true, `INSERT INTO users (username, email, realName, password_hash, salt, role, provider) VALUES (?, ?, ?, ?, ?, 'user', 'local')`,
		input.Username, input.Email, input.Name, hashedPassword, salt)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	token, err := utils.GenerateAccessToken(int(id), input.Email, input.Name, "user")
	if err != nil {
		http.Error(w, "Error generando token", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{
		"message": "Usuario creado correctamente",
		"token":   token,
	})
}

func (h *AuthHandler) GetUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Select(`SELECT id, username, role, email, realName, created_at, updated_at, provider FROM users`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var users []PublicUser
	for rows.Next() {
		var u PublicUser
		var createdAt, updatedAt string
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &u.Email, &u.Name, &createdAt, &updatedAt, &u.Provider); err != nil {
			continue
		}
		u.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		u.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)
		users = append(users, u)
	}
	json.NewEncoder(w).Encode(users)
}

func (h *AuthHandler) GetUserByID(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	row, err := h.DB.SelectRow(`SELECT id, username, role, email, realName, created_at, updated_at, provider FROM users WHERE id = ?`, userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var u PublicUser
	var createdAt, updatedAt string
	if err := row.Scan(&u.ID, &u.Username, &u.Role, &u.Email, &u.Name, &createdAt, &updatedAt, &u.Provider); err != nil {
		http.Error(w, "Usuario no encontrado", http.StatusNotFound)
		return
	}
	u.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	u.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)
	json.NewEncoder(w).Encode(u)
}
func (h *AuthHandler) LoginUser(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "Datos inválidos", http.StatusBadRequest)
		return
	}

	var user models.User
	row, err := h.DB.SelectRow(`
		SELECT id, email, username, realName, password_hash, salt, role
		FROM users
		WHERE email = ? AND provider = 'local'`, input.Email)
	if err != nil {
		http.Error(w, "Usuario no encontrado", http.StatusUnauthorized)
		return
	}
	err = row.Scan(&user.ID, &user.Email, &user.Username, &user.Name, &user.PasswordHash, &user.Salt, &user.Role)
	if err != nil {
		http.Error(w, "Credenciales inválidas", http.StatusUnauthorized)
		return
	}

	if !utils.ComparePasswords(user.PasswordHash, input.Password, user.Salt) {
		http.Error(w, "Contraseña incorrecta", http.StatusUnauthorized)
		return
	}

	token, err := utils.GenerateAccessToken(user.ID, user.Email, user.Name, user.Role)
	if err != nil {
		http.Error(w, "Error generando token", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{
		"token": token,
		"email": user.Email,
		"name":  user.Name,
		"rank":  user.Role,
	})
}

func (h *AuthHandler) CreateOrLoginWithProvider(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email    string `json:"email"`
		Name     string `json:"name"`
		Provider string `json:"provider"`
		Token    string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "Datos inválidos", http.StatusBadRequest)
		return
	}

	user, err := models.FindUserByEmailAndProvider(h.DB, input.Email, input.Provider)
	if err != nil && err != sql.ErrNoRows {
		log.Println("ERROR al buscar usuario por email y provider:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if user == nil {
		user = &models.User{
			Username: input.Email,
			Email:    input.Email,
			Name:     normalizeUTF8(input.Name),

			Role:     "user",
			Provider: input.Provider,
		}
		id, err := h.DB.Insert(true, `INSERT INTO users (username, email, realName, role, provider) VALUES (?, ?, ?, ?, ?)`,
			user.Username, user.Email, user.Name, user.Role, user.Provider)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		user.ID = int(id)
	}

	token, err := utils.GenerateAccessToken(user.ID, user.Email, user.Name, user.Role)
	if err != nil {
		http.Error(w, "Error generando token", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{
		"token": token,
		"email": user.Email,
		"name":  user.Name,
		"role":  user.Role,
	})
}
func (h *AuthHandler) GetUsersDatatable(w http.ResponseWriter, r *http.Request) {
	offsetParam := r.URL.Query().Get("offset")
	limitParam := r.URL.Query().Get("limit")
	search := r.URL.Query().Get("q")
	sortBy := r.URL.Query().Get("sort")
	order := r.URL.Query().Get("order")

	idFilter := r.URL.Query().Get("id")
	usernameFilter := r.URL.Query().Get("username")
	emailFilter := r.URL.Query().Get("email")
	roleFilter := r.URL.Query().Get("role")

	offset := 0
	limit := 10

	if offsetParam != "" {
		offset, _ = strconv.Atoi(offsetParam)
	}
	if limitParam != "" {
		limit, _ = strconv.Atoi(limitParam)
	}

	query := `
		SELECT id, username, role, email, realName, created_at, updated_at, provider
		FROM users
		WHERE 1=1
	`
	var queryParams []interface{}

	if search != "" {
		pattern := "%" + search + "%"
		query += ` AND (username LIKE ? OR email LIKE ? OR role LIKE ? OR realName LIKE ?)`
		queryParams = append(queryParams, pattern, pattern, pattern, pattern)
	}
	if idFilter != "" {
		query += " AND id LIKE ?"
		queryParams = append(queryParams, "%"+idFilter+"%")
	}
	if usernameFilter != "" {
		query += " AND username LIKE ?"
		queryParams = append(queryParams, "%"+usernameFilter+"%")
	}
	if emailFilter != "" {
		query += " AND email LIKE ?"
		queryParams = append(queryParams, "%"+emailFilter+"%")
	}
	if roleFilter != "" {
		query += " AND role LIKE ?"
		queryParams = append(queryParams, "%"+roleFilter+"%")
	}

	if sortBy != "" {
		validSorts := map[string]bool{"id": true, "username": true, "email": true, "role": true}
		if validSorts[sortBy] {
			if order != "desc" {
				order = "asc"
			}
			query += " ORDER BY " + sortBy + " " + strings.ToUpper(order)
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

	var users []PublicUser
	for rows.Next() {
		var u PublicUser
		var createdAt, updatedAt string
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &u.Email, &u.Name, &createdAt, &updatedAt, &u.Provider); err != nil {
			continue
		}
		u.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		u.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)
		users = append(users, u)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(users)
}
func (h *AuthHandler) GetUsersCount(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("q")
	idFilter := r.URL.Query().Get("id")
	usernameFilter := r.URL.Query().Get("username")
	emailFilter := r.URL.Query().Get("email")
	roleFilter := r.URL.Query().Get("role")

	query := `SELECT COUNT(*) FROM users WHERE 1=1`
	var queryParams []interface{}

	if search != "" {
		pattern := "%" + search + "%"
		query += ` AND (username LIKE ? OR email LIKE ? OR role LIKE ? OR realName LIKE ?)`
		queryParams = append(queryParams, pattern, pattern, pattern, pattern)
	}
	if idFilter != "" {
		query += " AND id LIKE ?"
		queryParams = append(queryParams, "%"+idFilter+"%")
	}
	if usernameFilter != "" {
		query += " AND username LIKE ?"
		queryParams = append(queryParams, "%"+usernameFilter+"%")
	}
	if emailFilter != "" {
		query += " AND email LIKE ?"
		queryParams = append(queryParams, "%"+emailFilter+"%")
	}
	if roleFilter != "" {
		query += " AND role LIKE ?"
		queryParams = append(queryParams, "%"+roleFilter+"%")
	}

	row, err := h.DB.SelectRow(query, queryParams...)
	if err != nil {
		http.Error(w, "Error al contar usuarios", http.StatusInternalServerError)
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

func (h *AuthHandler) CreateArtistLinkRequest(w http.ResponseWriter, r *http.Request) {
	var input struct {
		UserID   int    `json:"user_id"`
		ArtistID int    `json:"artist_id"`
		Rol      string `json:"rol"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "Datos inválidos", http.StatusBadRequest)
		return
	}

	// Validaciones mínimas
	if input.UserID == 0 || input.ArtistID == 0 || input.Rol == "" {
		http.Error(w, "Faltan campos requeridos", http.StatusBadRequest)
		return
	}

	// Insertar solicitud en la base
	_, err := h.DB.Insert(false, `
		INSERT INTO artist_links (user_id, artist_id, rol, status)
		VALUES (?, ?, ?, 'pending')
	`, input.UserID, input.ArtistID, input.Rol)

	if err != nil {
		http.Error(w, "Error al guardar la solicitud: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"message": "Solicitud enviada",
	})
}

func (h *AuthHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	_, err := h.DB.Delete(true, "DELETE FROM users WHERE id = ?", userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
