package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"brotecolectivo/handlers"
)

// InitRoutes configura y devuelve el router con todas las rutas de la API.
// Esta función es el punto de entrada principal para la configuración de endpoints.
//
// @title Brote Colectivo API
// @version 1.0
// @description API para la plataforma cultural Brote Colectivo
// @contact.name Soporte Brote Colectivo
// @contact.url https://brotecolectivo.com/contacto
// @license.name Propietario
// @BasePath /
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
func InitRoutes(authHandler *handlers.AuthHandler) *chi.Mux {
	r := chi.NewRouter()

	// Middlewares generales
	r.Use(CORSMiddleware)
	r.Use(SecurityHeaders)
	r.Use(middleware.Logger)

	// Root (protegido con rate limit)
	r.Group(func(r chi.Router) {
		r.Use(RateLimit)
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Acceso restringido", http.StatusUnauthorized)
		})
	})

	// Endpoint para verificar la versión de la API
	r.Get("/version", getVersion)

	// Endpoint para aprobación directa de submissions (usado en enlaces de WhatsApp)
	r.Get("/direct-approve/{id}", authHandler.DirectApprove)

	// Grupo de rutas para autenticación
	r.Route("/auth", func(r chi.Router) {
		r.Use(RateLimit)

		// Endpoints de autenticación
		r.Post("/login", authHandler.LoginUser)                          // Inicio de sesión tradicional
		r.Post("/provider-login", authHandler.CreateOrLoginWithProvider) // Inicio de sesión con proveedores externos
		r.Post("/register", authHandler.CreateUser)                      // Registro de nuevos usuarios
	})

	// Endpoints para recuperación de contraseña
	r.Post("/request-recovery", authHandler.RequestPasswordRecovery) // Solicitar recuperación de contraseña
	r.Post("/change-password", authHandler.ChangePassword)           // Cambiar contraseña con token

	// Grupo de rutas para bandas/artistas
	r.Route("/bands", func(r chi.Router) {
		// Endpoints auxiliares
		r.Get("/count", authHandler.GetBandsCount)           // Obtener conteo total de bandas
		r.Get("/table", authHandler.GetBandsDatatable)       // Datos para DataTables
		r.Post("/upload-image", authHandler.UploadBandImage) // Subir imagen de banda
		r.Get("/slug/{slug}", authHandler.CheckBandSlug)     // Verificar disponibilidad de slug
		r.With(AuthMiddleware).Post("/generate-bio", authHandler.GenerateArtistBio) // Generar biografía con IA

		// Ruta protegida con autenticación
		r.With(AuthMiddleware).Get("/user/{user_id}", authHandler.GetUserBands) // Obtener artistas vinculados a un usuario

		// CRUD principal
		r.Get("/", authHandler.GetBands)    // Listar todas las bandas
		r.Post("/", authHandler.CreateBand) // Crear nueva banda
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", authHandler.GetBandByID)   // Obtener detalles de banda
			r.Put("/", authHandler.UpdateBand)    // Actualizar banda
			r.Delete("/", authHandler.DeleteBand) // Eliminar banda
		})
		r.Get("/search", authHandler.SearchBands) // Buscar artistas
	})

	// Grupo de rutas para álbumes
	r.Route("/albums", func(r chi.Router) {
		r.Get("/", authHandler.GetAlbums)    // Listar todos los álbumes
		r.Post("/", authHandler.CreateAlbum) // Crear nuevo álbum
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", authHandler.GetAlbumByID)   // Obtener detalles de álbum
			r.Put("/", authHandler.UpdateAlbum)    // Actualizar álbum
			r.Delete("/", authHandler.DeleteAlbum) // Eliminar álbum
		})
	})

	// Grupo de rutas para eventos
	r.Route("/events", func(r chi.Router) {
		// Endpoints auxiliares
		r.Get("/count", authHandler.GetEventsCount)     // Obtener conteo total de eventos
		r.Get("/table", authHandler.GetEventsDatatable) // Datos para DataTables

		r.Get("/slug/{slug}", authHandler.CheckEventSlug) // Verificar disponibilidad de slug

		r.Get("/", authHandler.GetEvents)    // Listar todos los eventos
		r.Post("/upload-image", authHandler.UploadEventImage)
		r.With(AuthMiddleware).Post("/generate-description", authHandler.GenerateEventDescription)

		r.With(AuthMiddleware).Group(func(r chi.Router) {
			r.Post("/", authHandler.CreateEvent) // Crear nuevo evento

			r.Get("/user/{user_id}", authHandler.GetUserEvents) // Obtener eventos vinculados a un usuario

			r.Get("/band/{id}", authHandler.GetEventsByBandID)   // Eventos por banda
			r.Get("/venue/{id}", authHandler.GetEventsByVenueID) // Eventos por venue

			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", authHandler.GetEventByID)                                                   // Obtener detalles de evento
				r.Put("/", authHandler.UpdateEvent)                                                    // Actualizar evento
				r.Delete("/", authHandler.DeleteEvent)                                                 // Eliminar evento
				r.Get("/bands", authHandler.GetEventBands)                                             // Obtener bandas asociadas al evento
				r.With(AuthMiddleware).Post("/publish-instagram", authHandler.PublishEventToInstagram) // Publicar evento en Instagram
			})
		})
		r.Post("/{id}/publish-instagram", authHandler.PublishEventToInstagram) // Nueva ruta para publicar en Instagram
	})

	// Endpoint para solicitudes de vinculación de artistas
	r.Post("/artist-link-request", authHandler.CreateArtistLinkRequest)

	// Grupo de rutas para submissions (propuestas de contenido)
	r.Route("/submissions", func(r chi.Router) {
		r.Get("/", authHandler.GetSubmissions)                     // Listar todas las submissions
		r.Post("/upload-image", authHandler.UploadSubmissionImage) // Subir imagen para submission
		r.Post("/", authHandler.CreateSubmission)                  // Crear nueva submission
		r.Post("/generate-content", authHandler.GenerateNewsContent) // Generar contenido con IA
		r.Get("/{id}", authHandler.GetSubmissionByID)              // Obtener detalles de submission
		r.Post("/{id}/approve", authHandler.ApproveSubmission)     // Aprobar submission
		r.Put("/{id}", authHandler.UpdateSubmissionStatus)         // Actualizar estado de submission
	})

	// Grupo de rutas para ediciones (cambios propuestos a contenido existente)
	r.Route("/edits", func(r chi.Router) {
		r.Get("/", authHandler.GetEdits)             // Listar todas las ediciones
		r.Post("/", authHandler.CreateEdit)          // Crear nueva edición
		r.Get("/{id}", authHandler.GetEditByID)      // Obtener detalles de edición
		r.Put("/{id}", authHandler.UpdateEditStatus) // Actualizar estado de edición
	})

	// Grupo de rutas para noticias
	r.Route("/news", func(r chi.Router) {
		// Endpoints auxiliares
		r.Get("/count", authHandler.GetNewsCount)            // Obtener conteo total de noticias
		r.Post("/upload-image", authHandler.UploadNewsImage) // Subir imagen de noticia
		r.Get("/table", authHandler.GetNewsDatatable)        // Datos para DataTables
		r.Post("/generate-content", authHandler.GenerateNewsContent) // Generar contenido con IA

		// CRUD principal
		r.Get("/", authHandler.GetNews)     // Listar todas las noticias
		r.Post("/", authHandler.CreateNews) // Crear nueva noticia

		// Endpoints de relación
		r.Get("/band/{id}", authHandler.GetNewsByBandID) // Noticias por banda

		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", authHandler.GetNewsByID)   // Obtener detalles de noticia
			r.Put("/", authHandler.UpdateNews)    // Actualizar noticia
			r.Delete("/", authHandler.DeleteNews) // Eliminar noticia
		})
	})

	// Grupo de rutas para venues (lugares)
	r.Route("/venues", func(r chi.Router) {
		r.Get("/", authHandler.GetVenues)    // Listar todos los venues
		r.Post("/", authHandler.CreateVenue) // Crear nuevo venue
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", authHandler.GetVenueByIDOrSlug) // Obtener detalles de venue
			r.Put("/", authHandler.UpdateVenue)        // Actualizar venue
			r.Delete("/", authHandler.DeleteVenue)     // Eliminar venue
		})
		r.With(AuthMiddleware).Get("/user/{user_id}", authHandler.GetUserVenues) // Obtener venues vinculados a un usuario
	})

	// Grupo de rutas para videos
	r.Route("/videos", func(r chi.Router) {
		// Endpoints de relación
		r.Get("/band/{id}", authHandler.GetVideosByBandID) // Videos por banda

		// CRUD principal
		r.Get("/", authHandler.GetVideos)    // Listar todos los videos
		r.Post("/", authHandler.CreateVideo) // Crear nuevo video
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", authHandler.GetVideoByID)   // Obtener detalles de video
			r.Put("/", authHandler.UpdateVideo)    // Actualizar video
			r.Delete("/", authHandler.DeleteVideo) // Eliminar video
		})
	})

	// Grupo de rutas para canciones
	r.Route("/songs", func(r chi.Router) {
		// Endpoints específicos
		r.Get("/lyrics/{id}", authHandler.GetLyricsByID) // Obtener letras de canción

		// CRUD principal
		r.Get("/", authHandler.GetSongs)    // Listar todas las canciones
		r.Post("/", authHandler.CreateSong) // Crear nueva canción
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", authHandler.GetSongByID)   // Obtener detalles de canción
			r.Put("/", authHandler.UpdateSong)    // Actualizar canción
			r.Delete("/", authHandler.DeleteSong) // Eliminar canción
		})
	})

	// Grupo de rutas para usuarios
	r.Route("/users", func(r chi.Router) {
		r.Get("/count", authHandler.GetUsersCount)     // Obtener conteo total de usuarios
		r.Get("/table", authHandler.GetUsersDatatable) // Datos para DataTables
		r.Post("/", authHandler.CreateUser)            // Crear nuevo usuario
		r.Get("/", authHandler.GetUsers)               // Listar todos los usuarios
		r.Get("/{id}", authHandler.GetUserByID)        // Obtener detalles de usuario
		r.Delete("/{id}", authHandler.DeleteUser)      // Eliminar usuario
	})

	return r
}
