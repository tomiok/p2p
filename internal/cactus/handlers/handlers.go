package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"videocall/pkg/web"

	"github.com/go-chi/chi/v5"
	"videocall/internal/cactus"
)

type Handler struct {
	roomService  *cactus.RoomService
	tokenService *cactus.TokenService
}

func New(roomService *cactus.RoomService, tokenService *cactus.TokenService) *Handler {
	return &Handler{
		roomService:  roomService,
		tokenService: tokenService,
	}
}

// Web Handlers
func (h *Handler) Home(w http.ResponseWriter, r *http.Request) {
	// Check if there's a room parameter for direct join
	roomID := r.URL.Query().Get("room")

	w.Header().Set("Content-Type", "text/html")

	if roomID != "" {
		// Redirect to room page with room ID
		http.Redirect(w, r, fmt.Sprintf("/room?room=%s", roomID), http.StatusTemporaryRedirect)
		return
	}

	// Render home page
	// You'll handle the template rendering
	w.WriteHeader(http.StatusOK)
	web.Render(w, "index.page.tmpl", web.TemplateData{}, false)
}

func (h *Handler) Room(w http.ResponseWriter, r *http.Request) {
	// Get room ID from URL parameter or query
	roomID := chi.URLParam(r, "roomID")
	if roomID == "" {
		roomID = r.URL.Query().Get("room")
	}

	if roomID == "" {
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	// Validate room exists
	_, err := h.roomService.GetRoom(r.Context(), roomID)
	if err != nil {
		// Room doesn't exist, redirect to home with error
		http.Redirect(w, r, "/?error=room_not_found", http.StatusTemporaryRedirect)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)

	// Pass roomID to template
	web.Render(w, "room.page.tmpl", web.TemplateData{}, false)
}

// API Handlers
func (h *Handler) GetRoom(w http.ResponseWriter, r *http.Request) {
	roomID := chi.URLParam(r, "roomID")
	if roomID == "" {
		http.Error(w, "Room ID is required", http.StatusBadRequest)
		return
	}

	room, err := h.roomService.GetRoom(r.Context(), roomID)
	if err != nil {
		http.Error(w, "Room not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(room)
}

func (h *Handler) JoinRoom(w http.ResponseWriter, r *http.Request) {
	roomID := chi.URLParam(r, "roomID")
	if roomID == "" {
		http.Error(w, "Room ID is required", http.StatusBadRequest)
		return
	}

	var req cactus.JoinRoomRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}

	// Validate room exists
	room, err := h.roomService.GetRoom(r.Context(), roomID)
	if err != nil {
		http.Error(w, "Room not found", http.StatusNotFound)
		return
	}

	// Check if room has space
	if room.ActiveParticipants >= room.MaxParticipants {
		http.Error(w, "Room is full", http.StatusConflict)
		return
	}

	// Generate token
	token, err := h.tokenService.GenerateToken(roomID, req.Name)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to generate token: %v", err), http.StatusInternalServerError)
		return
	}

	response := &cactus.JoinRoomResponse{
		Token:      token,
		LiveKitURL: h.getLiveKitWSURL(),
		Room:       room,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *Handler) DeleteRoom(w http.ResponseWriter, r *http.Request) {
	roomID := chi.URLParam(r, "roomID")
	if roomID == "" {
		http.Error(w, "Room ID is required", http.StatusBadRequest)
		return
	}

	err := h.roomService.DeleteRoom(r.Context(), roomID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to delete room: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"status":    "ok",
		"timestamp": "2024-01-01T00:00:00Z", // You can use time.Now()
		"service":   "videocall-api",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Helper methods
func (h *Handler) getBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}

	host := r.Host
	if host == "" {
		host = "localhost:8080"
	}

	return fmt.Sprintf("%s://%s", scheme, host)
}

func (h *Handler) getLiveKitWSURL() string {
	// Convert HTTP URL to WebSocket URL if needed
	// For development, return the configured URL
	return "ws://localhost:7880" // This should come from config in production
}
