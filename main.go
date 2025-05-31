package main

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/joho/godotenv"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"videocall/internal/cactus"
	"videocall/internal/cactus/handlers"
	"videocall/internal/config"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}

	// Load configuration
	cfg := config.Load()

	// Initialize services
	roomService := cactus.NewRoomService(cfg)
	tokenService := cactus.NewTokenService(cfg)

	// Initialize handlers
	h := handlers.New(roomService, tokenService)

	// Setup router
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)

	// CORS
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	// Routes
	setupRoutes(r, h)

	port := cfg.Port
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on port %s", port)
	log.Printf("LiveKit URL: %s", cfg.LiveKitURL)

	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatal("Server failed to start:", err)
	}
}

func setupRoutes(r *chi.Mux, h *handlers.Handler) {
	fileServer(r)

	// Web pages
	r.Get("/", h.Home)
	r.Get("/room", h.Room)
	r.Get("/room/{roomID}", h.Room)

	// API routes
	r.Route("/api", func(r chi.Router) {
		r.Get("/rooms/{roomID}", h.GetRoom)
		r.Post("/rooms/{roomID}/join", h.JoinRoom)
		r.Delete("/rooms/{roomID}", h.DeleteRoom)

		// Health check
		r.Get("/health", h.Health)
	})

	// Static files (if needed)
	r.Get("/static/*", func(w http.ResponseWriter, r *http.Request) {
		http.StripPrefix("/static/", http.FileServer(http.Dir("./static/"))).ServeHTTP(w, r)
	})
}

// fs conveniently sets up a http.FileServer handler to serve
// static files from a http.FileSystem.
func fs(r chi.Router, path string, root http.FileSystem) {
	if strings.ContainsAny(path, "{}*") {
		panic("file server does not permit any URL parameters")
	}

	if path != "/" && path[len(path)-1] != '/' {
		r.Get(path, http.RedirectHandler(path+"/", http.StatusMovedPermanently).ServeHTTP)
		path += "/"
	}
	path += "*"

	r.Get(path, func(w http.ResponseWriter, r *http.Request) {
		rctx := chi.RouteContext(r.Context())
		pathPrefix := strings.TrimSuffix(rctx.RoutePattern(), "/*")
		h := http.StripPrefix(pathPrefix, http.FileServer(root))
		h.ServeHTTP(w, r)
	})
}

func fileServer(r chi.Router) {
	workDir, _ := os.Getwd()
	filesDir := http.Dir(filepath.Join(workDir, "static"))
	fs(r, "/static", filesDir)
}
