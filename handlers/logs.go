package handlers

import (
	"brotecolectivo/utils"
	"fmt"
	"net/http"
)

func (h *AuthHandler) InsertLog(logType, oldValue, newValue string, r *http.Request) error {
	user, err := utils.GetCurrentUser(r, h.DB) // Asumo que getCurrentUser devuelve (*User, error)
	if err != nil {
		// Maneja el error de no poder obtener el usuario, puede que no esté autorizado o el token no sea válido
		return err
	}

	// Asegúrate de que user no es nil para evitar panic
	if user == nil {
		return fmt.Errorf("usuario no encontrado o no autorizado")
	}

	// Preparar la sentencia SQL para insertar el registro
	_, err = h.DB.Insert(true, "INSERT INTO logs (`type`, `old_value`, `new_value`, `user_id`) VALUES (?, ?, ?, ?)", logType, oldValue, newValue, user.ID)
	return err

}
