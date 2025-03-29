package main

import (
	"brotecolectivo/models"
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt"
	"golang.org/x/time/rate"
)

var limiter = rate.NewLimiter(1, 3) // Permite 1 solicitud por segundo con un burst de 3.

func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Obtenemos el token de autorización del encabezado
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "No autorizado. Token no proporcionado.", http.StatusUnauthorized)
			return
		}

		// Verificamos si el token es "Bearer token-secreto" y lo salteamos
		if authHeader == "Bearer token-secreto" {
			// Para depuración, crear claims temporales
			fmt.Println("[DEBUG] Usando token de depuración")
			tempClaims := &models.Claims{
				UserID: 1,       // ID de usuario por defecto para depuración
				Role:   "admin", // Rol por defecto para depuración
			}
			ctx := context.WithValue(r.Context(), "user", tempClaims)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// El token debe estar en el formato "Bearer {token}"
		tokenParts := strings.Split(authHeader, " ")
		if len(tokenParts) != 2 || tokenParts[0] != "Bearer" {
			http.Error(w, "No autorizado. Formato de token inválido.", http.StatusUnauthorized)
			return
		}

		// Parseamos y validamos el token
		tokenString := tokenParts[1]
		claims := &models.Claims{}

		fmt.Printf("[DEBUG] Intentando validar token: %s\n", tokenString[:10]+"...")

		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			fmt.Printf("[DEBUG] Método de firma del token: %v\n", token.Method)
			return jwtKey, nil
		})

		if err != nil {
			fmt.Printf("[DEBUG] Error al validar token: %v\n", err)
			if err == jwt.ErrSignatureInvalid {
				http.Error(w, "No autorizado. Token de autenticación inválido.", http.StatusUnauthorized)
				return
			}
			http.Error(w, "No autorizado. Token de autenticación inválido.", http.StatusUnauthorized)
			return
		}
		if !token.Valid {
			fmt.Printf("[DEBUG] Token inválido\n")
			http.Error(w, "No autorizado. Token de autenticación inválido.", http.StatusUnauthorized)
			return
		}

		fmt.Printf("[DEBUG] Token válido para usuario ID: %d, Rol: %s\n", claims.UserID, claims.Role)

		// Si el token es válido, pasamos al siguiente middleware o controlador
		// Guardar los claims en el contexto para que los controladores puedan acceder a ellos
		ctx := context.WithValue(r.Context(), "user", claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func SecurityHeaders(next http.Handler) http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		w.Header().Set("X-XSS-Protection", "1; mode=block")

		next.ServeHTTP(w, r)
	})
}

func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "http://localhost:8080" || origin == "https://brotecolectivo.com" || origin == "https://www.brotecolectivo.com" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func RateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !limiter.Allow() {
			http.Error(w, "Demasiadas solicitudes, intenta de nuevo más tarde.", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}
