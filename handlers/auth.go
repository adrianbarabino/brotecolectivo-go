package handlers

import (
	"brotecolectivo/database"
	"brotecolectivo/models"
	"brotecolectivo/utils"
	"crypto/md5"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

type AuthHandler struct {
	DB *database.DatabaseStruct
}

func NewAuthHandler(db *database.DatabaseStruct) *AuthHandler {
	return &AuthHandler{DB: db}
}

func (h *AuthHandler) RequestPasswordRecovery(w http.ResponseWriter, r *http.Request) {
	var requestData struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&requestData); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var u models.User
	row, _ := h.DB.SelectRow("SELECT id, email FROM users WHERE email = ?", requestData.Email)
	err := row.Scan(&u.ID, &u.Email)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Email no encontrado", http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	recoveryToken := fmt.Sprintf("%x", md5.Sum([]byte(time.Now().String()+u.Email)))
	_, err = h.DB.Update(true, "UPDATE users SET recovery_hash = ?, recovery_hash_time = NOW() WHERE id = ?", recoveryToken, u.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = utils.SendRecoveryEmail(u.Email, recoveryToken)
	if err != nil {
		log.Println("Error al enviar el correo de recuperación:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "Instrucciones de recuperación enviadas."})
}

func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	var requestData struct {
		Token       string `json:"token"`
		NewPassword string `json:"newPassword"`
	}
	if err := json.NewDecoder(r.Body).Decode(&requestData); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var userID int
	var recoveryHashTimeStr string
	row, _ := h.DB.SelectRow("SELECT id, recovery_hash_time FROM users WHERE recovery_hash = ?", requestData.Token)
	err := row.Scan(&userID, &recoveryHashTimeStr)
	if err != nil {
		log.Println("Error al obtener token de recuperación:", err)
		http.Error(w, "Error al obtener el token de recuperación", http.StatusInternalServerError)
		return
	}

	recoveryHashTime, err := time.Parse("2006-01-02 15:04:05", recoveryHashTimeStr)
	if err != nil {
		log.Println("Error al parsear recoveryHashTime:", err)
		http.Error(w, "Error al procesar la fecha del token", http.StatusInternalServerError)
		return
	}

	if time.Since(recoveryHashTime).Hours() > 24 {
		http.Error(w, "El token de recuperación ha expirado", http.StatusBadRequest)
		return
	}

	newSalt := utils.GenerateSalt()
	newHashedPassword := utils.HashPassword(requestData.NewPassword, newSalt)
	_, err = h.DB.Update(true, "UPDATE users SET password_hash = ?, salt = ?, recovery_hash = NULL WHERE id = ?", newHashedPassword, newSalt, userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "Contraseña actualizada con éxito."})
}
