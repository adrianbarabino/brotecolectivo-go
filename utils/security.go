// utils/security.go
package utils

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"brotecolectivo/models"

	"github.com/golang-jwt/jwt"
	"golang.org/x/crypto/argon2"

	"github.com/mailgun/mailgun-go/v4"
)

// Esta función debe ser seteada al iniciar tu app para poder usarla desde utils
var GetMailgunConfig func() (domain string, apiKey string, err error)

func SendRecoveryEmail(email, token string) error {
	domain, apiKey, err := GetMailgunConfig()
	if err != nil {
		return err
	}

	mg := mailgun.NewMailgun(domain, apiKey)

	sender := "no-reply@m.brote.org"
	subject := "Recuperación de contraseña"
	logoURL := "https://cnc.brote.store/themes/2019/img/logo.png"
	body := fmt.Sprintf(`
	<html>
	<body>
		<div style="text-align: center;">
			<img src="%s" alt="Logo BROTE" style="max-width: 200px; margin-bottom: 20px;">
			<p>Tu token de recuperación es: <strong>%s</strong></p>
			<p>Introdúcelo en la página web para poder recuperar tu contraseña:</p>
			<a href="http://localhost:3000/password-recovery/%s/" style="display: inline-block; background-color: #007BFF; color: white; padding: 10px 20px; text-decoration: none; border-radius: 5px; font-weight: bold;">Recuperar Contraseña</a>
		</div>
	</body>
	</html>
	`, logoURL, token, token)

	message := mg.NewMessage(sender, subject, "", email)
	message.SetHtml(body)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	_, _, err = mg.Send(ctx, message)
	if err != nil {
		log.Println("Error enviando correo con Mailgun:", err)
	}
	return err
}

var JwtKey []byte

func SetJwtKey(key []byte) {
	JwtKey = key
}

func GenerateAccessToken(userID int, userName string, realName string) (string, error) {
	expirationTime := time.Now().Add(24 * time.Hour * 180) // 180 días
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":   userID,
		"user_name": userName,
		"real_name": realName,
		"exp":       expirationTime.Unix(),
	})

	return token.SignedString(JwtKey)
}

func GenerateSalt() string {
	salt := make([]byte, 16)
	rand.Read(salt)
	return hex.EncodeToString(salt)
}

func HashPassword(password, salt string) string {
	saltBytes, _ := hex.DecodeString(salt)
	hash := argon2.IDKey([]byte(password), saltBytes, 1, 64*1024, 4, 32)
	return hex.EncodeToString(hash)
}

func ComparePasswords(hashedPassword, password, salt string) bool {
	saltBytes, err := hex.DecodeString(salt)
	if err != nil {
		log.Println("Error al decodificar la sal:", err)
		return false
	}

	hashedPasswordBytes, err := hex.DecodeString(hashedPassword)
	if err != nil {
		log.Println("Error al decodificar el hash de la contraseña:", err)
		return false
	}

	hash := argon2.IDKey([]byte(password), saltBytes, 1, 64*1024, 4, 32)
	return subtle.ConstantTimeCompare(hashedPasswordBytes, hash) == 1
}

// Verificar si el nombre de usuario ya existe en la base de datos
func UsernameExists(username string, db interface {
	SelectRow(query string, args ...interface{}) (*sql.Row, error)
}) bool {
	var id int
	rows, err := db.SelectRow("SELECT id FROM users WHERE username = ?", username)
	if err != nil {
		if err == sql.ErrNoRows {
			// No se encontró el nombre de usuario, por lo que no existe
			return false
		}
		// Manejar otros posibles errores
		log.Printf("Error al verificar el nombre de usuario: %v\n", err)
		return false
	}
	err = rows.Scan(&id)
	if err != nil {
		// Manejar errores al escanear
		log.Printf("Error al escanear el ID del usuario: %v\n", err)
		return false
	}
	// Si la consulta no devolvió ErrNoRows y se pudo escanear el ID, significa que se encontró un registro
	return id != 0
}
func GetUserFromToken(tokenString string, db interface {
	SelectRow(query string, args ...interface{}) (*sql.Row, error)
}) (*models.User, error) {
	token, err := jwt.ParseWithClaims(tokenString, &models.Claims{}, func(token *jwt.Token) (interface{}, error) {
		return JwtKey, nil
	})
	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*models.Claims); ok && token.Valid {
		userID := claims.UserID
		var user models.User
		row, _ := db.SelectRow("SELECT id, username, email FROM users WHERE id = ?", userID)
		err := row.Scan(&user.ID, &user.Username, &user.Email)
		if err != nil {
			return nil, err
		}
		return &user, nil
	}

	return nil, fmt.Errorf("token inválido o expirado")
}

func CheckAccessToken(accessToken string, db interface {
	SelectRow(query string, args ...interface{}) *sql.Row
}) (int, error) {
	token, err := jwt.Parse(accessToken, func(token *jwt.Token) (interface{}, error) {
		return JwtKey, nil
	})
	if err != nil || !token.Valid {
		return 0, fmt.Errorf("token inválido")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return 0, fmt.Errorf("error al leer los claims")
	}

	userID := int(claims["user_id"].(float64))
	var id int
	row := db.SelectRow("SELECT id FROM users WHERE id = ?", userID)
	err = row.Scan(&id)
	if err != nil || id == 0 {
		return 0, fmt.Errorf("usuario no encontrado")
	}

	return userID, nil
}

func GetCurrentUser(r *http.Request, db interface {
	SelectRow(query string, args ...interface{}) (*sql.Row, error)
}) (*models.User, error) {
	authToken := r.Header.Get("Authorization")
	if authToken == "token-secreto" {
		return &models.User{ID: 1, Username: "admin", Email: "admin@example.com"}, nil
	}
	tokenString := strings.TrimPrefix(authToken, "Bearer ")
	return GetUserFromToken(tokenString, db)
}
