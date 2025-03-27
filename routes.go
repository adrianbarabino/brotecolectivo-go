package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"brotecolectivo/handlers"
)

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

	r.Get("/version", getVersion)

	r.Route("/auth", func(r chi.Router) {
		r.Use(RateLimit)

		r.Post("/login", authHandler.LoginUser)                          // POST /auth/login
		r.Post("/provider-login", authHandler.CreateOrLoginWithProvider) // POST /auth/provider-login
		r.Post("/register", authHandler.CreateUser)                      // POST /auth/register
	})

	r.Post("/request-recovery", authHandler.RequestPasswordRecovery)
	r.Post("/change-password", authHandler.ChangePassword)

	// Rutas públicas (RESTful)
	r.Route("/bands", func(r chi.Router) {
		r.Get("/count", authHandler.GetBandsCount)
		r.Get("/table", authHandler.GetBandsDatatable)
		r.Post("/upload-image", authHandler.UploadBandImage)
		r.Get("/slug/{slug}", authHandler.CheckBandSlug)

		r.Get("/", authHandler.GetBands)
		r.Post("/", authHandler.CreateBand)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", authHandler.GetBandByID)
			r.Put("/", authHandler.UpdateBand)
			r.Delete("/", authHandler.DeleteBand)
		})
	})

	r.Route("/albums", func(r chi.Router) {
		r.Get("/", authHandler.GetAlbums)
		r.Post("/", authHandler.CreateAlbum)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", authHandler.GetAlbumByID)
			r.Put("/", authHandler.UpdateAlbum)
			r.Delete("/", authHandler.DeleteAlbum)
		})
	})

	// r.Route("/songs", func(r chi.Router) {
	// 	r.Get("/", handlers.GetSongs)
	// 	r.Post("/", handlers.CreateSong)
	// 	r.Route("/{id}", func(r chi.Router) {
	// 		r.Get("/", handlers.GetSongByID)
	// 		r.Put("/", handlers.UpdateSong)
	// 		r.Delete("/", handlers.DeleteSong)
	// 	})
	// })

	// r.Route("/videos", func(r chi.Router) {
	// 	r.Get("/", handlers.GetVideos)
	// 	r.Post("/", handlers.CreateVideo)
	// 	r.Route("/{id}", func(r chi.Router) {
	// 		r.Get("/", handlers.GetVideoByID)
	// 		r.Put("/", handlers.UpdateVideo)
	// 		r.Delete("/", handlers.DeleteVideo)
	// 	})
	// })

	r.Route("/events", func(r chi.Router) {
		r.Get("/count", authHandler.GetEventsCount)
		r.Get("/table", authHandler.GetEventsDatatable)
		r.Post("/upload-image", authHandler.UploadEventImage)
		r.Get("/", authHandler.GetEvents)    // Todos los eventos (con búsqueda, paginación, etc)
		r.Post("/", authHandler.CreateEvent) // Crear evento con bandas

		r.Get("/band/{id}", authHandler.GetEventsByBandID)   // Eventos por banda
		r.Get("/venue/{id}", authHandler.GetEventsByVenueID) // Eventos por venue

		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", authHandler.GetEventByID)   // Detalle evento
			r.Put("/", authHandler.UpdateEvent)    // Actualizar evento y bandas
			r.Delete("/", authHandler.DeleteEvent) // Borrar evento + bandas asociadas
		})
	})
	r.Post("/artist-link-request", authHandler.CreateArtistLinkRequest)

	r.Route("/submissions", func(r chi.Router) {
		r.Get("/", authHandler.GetSubmissions) // Obtener todas las submissions (solo admin o moderador)
		r.Post("/upload-image", authHandler.UploadSubmissionImage)
		r.Post("/", authHandler.CreateSubmission)     // Crear nueva submission (cualquier usuario)
		r.Get("/{id}", authHandler.GetSubmissionByID) // Ver una submission individual
		r.Post("/{id}/approve", authHandler.ApproveSubmission)
		r.Put("/{id}", authHandler.UpdateSubmissionStatus) // Aprobar/Rechazar una submission (admin/moderador)
	})
	r.Route("/edits", func(r chi.Router) {
		r.Get("/", authHandler.GetEdits)
		r.Post("/", authHandler.CreateEdit)
		r.Get("/{id}", authHandler.GetEditByID)
		r.Put("/{id}", authHandler.UpdateEditStatus)
	})

	r.Route("/news", func(r chi.Router) {
		r.Get("/count", authHandler.GetNewsCount)
		r.Post("/upload-image", authHandler.UploadNewsImage)
		r.Get("/table", authHandler.GetNewsDatatable)
		r.Get("/", authHandler.GetNews)

		r.Post("/", authHandler.CreateNews)
		r.Get("/band/{id}", authHandler.GetNewsByBandID) // Eventos por banda

		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", authHandler.GetNewsByID)
			r.Put("/", authHandler.UpdateNews)
			r.Delete("/", authHandler.DeleteNews)
		})
	})
	r.Route("/venues", func(r chi.Router) {
		r.Get("/", authHandler.GetVenues)
		r.Post("/", authHandler.CreateVenue)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", authHandler.GetVenueByIDOrSlug)
			r.Put("/", authHandler.UpdateVenue)
			r.Delete("/", authHandler.DeleteVenue)
		})
	})

	// routes for videos
	r.Route("/videos", func(r chi.Router) {
		r.Get("/band/{id}", authHandler.GetVideosByBandID)
		r.Get("/", authHandler.GetVideos)
		r.Post("/", authHandler.CreateVideo)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", authHandler.GetVideoByID)
			r.Put("/", authHandler.UpdateVideo)
			r.Delete("/", authHandler.DeleteVideo)
		})
	})

	// for songs
	r.Route("/songs", func(r chi.Router) {
		// for Lyrics by ID
		r.Get("/lyrics/{id}", authHandler.GetLyricsByID)

		r.Get("/", authHandler.GetSongs)
		r.Post("/", authHandler.CreateSong)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", authHandler.GetSongByID)
			r.Put("/", authHandler.UpdateSong)
			r.Delete("/", authHandler.DeleteSong)
		})
	})

	// r.Route("/programs", func(r chi.Router) {
	// 	r.Get("/", handlers.GetPrograms)
	// 	r.Post("/", handlers.CreateProgram)
	// 	r.Route("/{id}", func(r chi.Router) {
	// 		r.Get("/", handlers.GetProgramByID)
	// 		r.Put("/", handlers.UpdateProgram)
	// 		r.Delete("/", handlers.DeleteProgram)
	// 	})
	// })

	// r.Route("/news", func(r chi.Router) {
	// 	r.Get("/", handlers.GetNews)
	// 	r.Post("/", handlers.CreateNews)
	// 	r.Route("/{id}", func(r chi.Router) {
	// 		r.Get("/", handlers.GetNewsByID)
	// 		r.Put("/", handlers.UpdateNews)
	// 		r.Delete("/", handlers.DeleteNews)
	// 	})
	// })

	// r.Route("/genres", func(r chi.Router) {
	// 	r.Get("/", handlers.GetGenres)
	// 	r.Post("/", handlers.CreateGenre)
	// 	r.Route("/{id}", func(r chi.Router) {
	// 		r.Get("/", handlers.GetGenreByID)
	// 		r.Put("/", handlers.UpdateGenre)
	// 		r.Delete("/", handlers.DeleteGenre)
	// 	})
	// })

	// r.Route("/contacts", func(r chi.Router) {
	// 	r.Get("/", handlers.GetContacts)
	// 	r.Post("/", handlers.CreateContact)
	// 	r.Route("/{id}", func(r chi.Router) {
	// 		r.Get("/", handlers.GetContactByID)
	// 		r.Put("/", handlers.UpdateContact)
	// 		r.Delete("/", handlers.DeleteContact)
	// 	})
	// })

	// r.Route("/newsletter", func(r chi.Router) {
	// 	r.Get("/", handlers.GetNewsletters)
	// 	r.Post("/", handlers.CreateNewsletter)
	// 	r.Route("/{id}", func(r chi.Router) {
	// 		r.Get("/", handlers.GetNewsletterByID)
	// 		r.Put("/", handlers.UpdateNewsletter)
	// 		r.Delete("/", handlers.DeleteNewsletter)
	// 	})
	// })

	// Grupo protegido con AuthMiddleware (si más adelante querés usuarios logueados)
	// r.Group(func(r chi.Router) {
	//     r.Use(AuthMiddleware)
	//     // Rutas protegidas
	// })
	r.Route("/users", func(r chi.Router) {
		r.Post("/", authHandler.CreateUser)
		r.Get("/", authHandler.GetUsers)
		r.Get("/{id}", authHandler.GetUserByID)
		r.Delete("/{id}", authHandler.DeleteUser)
	})

	return r
}
