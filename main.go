// main.go
package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type Server struct {
	*http.Server
}

// Start runs ListenAndServe on the http.Server with graceful shutdown.
func (s *Server) Start() {
	slog.Info("server starting", "port", s.Addr)
	go func() {
		if err := s.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		}
	}()
	s.gracefulShutdown()
}

func (s *Server) gracefulShutdown() {
	quit := make(chan os.Signal, 1)

	signal.Notify(quit, syscall.SIGINT)
	sig := <-quit

	slog.Warn("stopping server", "sig", sig)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	s.SetKeepAlivesEnabled(false)
	if err := s.Shutdown(ctx); err != nil {
	}
}

func newServer(port string, r chi.Router) *Server {
	srv := &http.Server{
		Addr:         ":" + port,
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
		Handler:      r,
	}

	return &Server{srv}
}

func main() {
	// Configuración del servidor
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Configuración del signaling server - CORREGIDO para tu servidor
	signalingURL := os.Getenv("SIGNALING_URL")
	if signalingURL == "" {
		signalingURL = "ws://localhost:8081/room" // Cambiado de /ws a /room
	}

	r := chi.NewRouter()

	// Servir páginas
	r.Get("/", serveIndex)
	r.Get("/room/{roomId}", serveRoom)

	// Servir archivos estáticos desde tu carpeta static/
	fileServer(r)

	// API endpoints
	r.Get("/api/config", getConfig(signalingURL))
	r.Post("/api/room", createRoom)
	r.Get("/api/room/{roomId}/info", getRoomInfo)

	log.Printf("Servidor web corriendo en puerto %s", port)
	log.Printf("Signaling server configurado en: %s", signalingURL)

	srv := newServer(port, r)
	srv.Start()
}

func serveIndex(w http.ResponseWriter, r *http.Request) {
	STR(w, 200, getIndexHTML())
}

func serveRoom(w http.ResponseWriter, r *http.Request) {
	roomId := chi.URLParam(r, "roomId")
	STR(w, 200, getRoomHTML(roomId))
}

func fileServer(r chi.Router) {
	workDir, _ := os.Getwd()
	filesDir := http.Dir(filepath.Join(workDir, "static"))
	fs(r, "/static", filesDir)
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

func getConfig(signalingURL string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		config := map[string]interface{}{
			"signalingUrl": signalingURL,
			"stunServers": []string{
				"stun:stun.l.google.com:19302",
				"stun:stun1.l.google.com:19302",
			},
		}
		JSON(w, 200, config)
	}
}

func createRoom(w http.ResponseWriter, r *http.Request) {
	// Generar UUID para la sala
	roomId := generateRoomId()

	room := map[string]interface{}{
		"id":           roomId,
		"url":          "/room/" + roomId,
		"createdAt":    time.Now().Unix(),
		"participants": 0,
	}

	JSON(w, 200, room)
}

func getRoomInfo(w http.ResponseWriter, r *http.Request) {
	roomId := chi.URLParam(r, "roomId")

	// Aquí podrías consultar info de la sala desde el signaling server
	// Por ahora, respuesta mock
	room := map[string]interface{}{
		"id":           roomId,
		"exists":       true,
		"participants": 0, // Se podría obtener del signaling server
	}

	JSON(w, 200, room)
}

func generateRoomId() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return fmt.Sprintf("%x", bytes)
}

func JSON[T any](w http.ResponseWriter, status int, t T) {
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(t)
}

func STR(w http.ResponseWriter, status int, t string) {
	w.Header().Add("Content-Type", "text/html")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(t))
}

func getIndexHTML() string {
	return `<!DOCTYPE html>
<html lang="es">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>VideoP2P - Videollamadas Directas</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body { 
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
        }
        .container {
            background: white;
            padding: 3rem;
            border-radius: 12px;
            box-shadow: 0 20px 40px rgba(0,0,0,0.1);
            text-align: center;
            max-width: 400px;
            width: 90%;
        }
        h1 { color: #333; margin-bottom: 1rem; }
        p { color: #666; margin-bottom: 2rem; }
        button {
            background: #667eea;
            color: white;
            border: none;
            padding: 12px 24px;
            border-radius: 6px;
            font-size: 16px;
            cursor: pointer;
            transition: background 0.2s;
            width: 100%;
        }
        button:hover { background: #5a6fd8; }
        .link-container {
            margin-top: 2rem;
            padding: 1rem;
            background: #f8f9fa;
            border-radius: 6px;
            display: none;
        }
        .room-link {
            word-break: break-all;
            background: white;
            padding: 8px 12px;
            border-radius: 4px;
            border: 1px solid #ddd;
            margin: 8px 0;
        }
        .copy-btn {
            background: #28a745;
            margin-top: 8px;
            padding: 8px 16px;
            font-size: 14px;
        }
        .copy-btn:hover { background: #218838; }
    </style>
</head>
<body>
    <div class="container">
        <h1>🎥 VideoP2P</h1>
        <p>Videollamadas directas sin apps ni registros</p>
        
        <button onclick="createRoom()">Crear Nueva Videollamada</button>
        
        <div id="linkContainer" class="link-container">
            <p><strong>¡Sala creada!</strong></p>
            <p>Comparte este enlace:</p>
            <div id="roomLink" class="room-link"></div>
            <button class="copy-btn" onclick="copyLink()">Copiar Enlace</button>
        </div>
    </div>

    <script>
        let currentRoomUrl = '';

        async function createRoom() {
            try {
                const response = await fetch('/api/room', { method: 'POST' });
                const room = await response.json();
                
                currentRoomUrl = window.location.origin + room.url;
                document.getElementById('roomLink').textContent = currentRoomUrl;
                document.getElementById('linkContainer').style.display = 'block';
            } catch (error) {
                alert('Error creando la sala: ' + error.message);
            }
        }

        function copyLink() {
            navigator.clipboard.writeText(currentRoomUrl).then(() => {
                const btn = document.querySelector('.copy-btn');
                const original = btn.textContent;
                btn.textContent = '¡Copiado!';
                setTimeout(() => btn.textContent = original, 2000);
            });
        }
    </script>
</body>
</html>`
}

func getRoomHTML(roomId string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="es">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>VideoP2P - Sala %s</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
            background: #1a1a1a;
            height: 100vh;
            display: flex;
            flex-direction: column;
        }
        .header {
            background: #2d2d2d;
            padding: 1rem;
            color: white;
            text-align: center;
            border-bottom: 1px solid #444;
        }
        .video-container {
            flex: 1;
            display: grid;
            gap: 10px;
            padding: 10px;
            grid-template-columns: 1fr;
            grid-template-rows: 1fr;
        }
        
        /* Layout para 1 persona */
        .video-container.participants-1 {
            grid-template-columns: 1fr;
            grid-template-rows: 1fr;
        }
        
        /* Layout para 2 personas */
        .video-container.participants-2 {
            grid-template-columns: 1fr 1fr;
            grid-template-rows: 1fr;
        }
        
        /* Layout para 3-4 personas */
        .video-container.participants-3,
        .video-container.participants-4 {
            grid-template-columns: 1fr 1fr;
            grid-template-rows: 1fr 1fr;
        }
        
        /* Layout para 5-6 personas */
        .video-container.participants-5,
        .video-container.participants-6 {
            grid-template-columns: 1fr 1fr 1fr;
            grid-template-rows: 1fr 1fr;
        }
        
        /* Layout para 7+ personas */
        .video-container.participants-7,
        .video-container.participants-8,
        .video-container.participants-9 {
            grid-template-columns: 1fr 1fr 1fr;
            grid-template-rows: 1fr 1fr 1fr;
        }
        
        .video-wrapper {
            position: relative;
            background: #000;
            border-radius: 8px;
            overflow: hidden;
            min-height: 200px;
            border: 2px solid transparent;
            transition: border-color 0.3s ease;
        }
        
        .video-wrapper.speaking {
            border-color: #28a745;
            box-shadow: 0 0 15px rgba(40, 167, 69, 0.5);
        }
        
        .video-wrapper.local {
            border-color: #667eea;
        }
        
        video {
            width: 100%%;
            height: 100%%;
            object-fit: cover;
        }
        
        .video-label {
            position: absolute;
            bottom: 10px;
            left: 10px;
            background: rgba(0,0,0,0.8);
            color: white;
            padding: 4px 8px;
            border-radius: 4px;
            font-size: 12px;
            font-weight: 500;
        }
        
        .video-status {
            position: absolute;
            top: 10px;
            right: 10px;
            display: flex;
            gap: 5px;
        }
        
        .status-icon {
            background: rgba(0,0,0,0.8);
            color: white;
            padding: 4px;
            border-radius: 4px;
            font-size: 12px;
        }
        
        .muted { color: #dc3545; }
        .cam-off { color: #ffc107; }
        
        /* Responsive design */
        @media (max-width: 768px) {
            .video-container.participants-2,
            .video-container.participants-3,
            .video-container.participants-4 {
                grid-template-columns: 1fr;
                grid-template-rows: repeat(auto, 1fr);
            }
            
            .video-container.participants-5,
            .video-container.participants-6,
            .video-container.participants-7,
            .video-container.participants-8,
            .video-container.participants-9 {
                grid-template-columns: 1fr 1fr;
                grid-template-rows: repeat(auto, 1fr);
            }
        }
        .controls {
            background: #2d2d2d;
            padding: 1rem;
            display: flex;
            justify-content: center;
            gap: 1rem;
        }
        .control-btn {
            background: #444;
            color: white;
            border: none;
            padding: 12px;
            border-radius: 50%%;
            cursor: pointer;
            width: 50px;
            height: 50px;
            display: flex;
            align-items: center;
            justify-content: center;
            transition: background 0.2s;
        }
        .control-btn:hover { background: #555; }
        .control-btn.active { background: #dc3545; }
        .status {
            color: white;
            text-align: center;
            padding: 1rem;
        }
        .connecting { color: #ffc107; }
        .connected { color: #28a745; }
        .error { color: #dc3545; }
    </style>
</head>
<body>
    <div class="header">
        <h2>Sala: %s</h2>
        <div id="status" class="status connecting">Conectando...</div>
    </div>
    
    <div class="video-container" id="videoContainer">
        <!-- Los videos se agregarán dinámicamente aquí -->
    </div>
    
    <div class="controls">
        <button id="micBtn" class="control-btn" onclick="toggleMic()">🎤</button>
        <button id="camBtn" class="control-btn" onclick="toggleCam()">📹</button>
        <button id="hangupBtn" class="control-btn" onclick="hangup()">📞</button>
    </div>

    <script src="/static/webrtc.js"></script>
    <script>
        const roomId = '%s';
        const videoCall = new VideoCall(roomId);
        videoCall.init();
    </script>
</body>
</html>`, roomId, roomId, roomId)
}
