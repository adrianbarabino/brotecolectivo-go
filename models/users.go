// models/user.go
package models

import (
	"brotecolectivo/database"
	"database/sql"
)

type User struct {
	ID           int    `json:"id"`
	Username     string `json:"username"`
	Email        string `json:"email"`
	Name         string `json:"realName"`
	Role         string `json:"role"`
	Provider     string `json:"provider"`
	PasswordHash string `json:"-"` // nunca se envía
	Salt         string `json:"-"` // nunca se envía
}

// Busca usuario por email y proveedor (Google, Microsoft, etc.)
func FindUserByEmailAndProvider(db *database.DatabaseStruct, email, provider string) (*User, error) {
	row, err := db.Select(`SELECT id, username, email, realName, role, provider FROM users WHERE email = ? AND provider = ?`, email, provider)
	if err != nil {
		return nil, err
	}
	defer row.Close()

	if row.Next() {
		var u User
		err := row.Scan(&u.ID, &u.Username, &u.Email, &u.Name, &u.Role, &u.Provider)
		if err != nil {
			return nil, err
		}
		return &u, nil
	}

	return nil, sql.ErrNoRows
}
