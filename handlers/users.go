package handlers

import (
	"brotecolectivo/models"
	"brotecolectivo/utils"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
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

	id, err := h.DB.Insert(true, `INSERT INTO users (username, email, realName, password_hash, salt, role, provider) VALUES (?, ?, ?, ?, ?, 'usuario', 'local')`,
		input.Username, input.Email, input.Name, hashedPassword, salt)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	token, err := utils.GenerateAccessToken(int(id), input.Email, input.Name)
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

	token, err := utils.GenerateAccessToken(user.ID, user.Email, user.Name)
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

			Role:     "usuario",
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

	token, err := utils.GenerateAccessToken(user.ID, user.Email, user.Name)
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

func (h *AuthHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	_, err := h.DB.Delete(true, "DELETE FROM users WHERE id = ?", userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
