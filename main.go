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
        button:disabled { 
            background: #ccc; 
            cursor: not-allowed; 
        }
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
        
        /* === ESTILOS PARA FORMULARIO DE NOMBRE === */
        .name-form {
            margin-bottom: 1rem;
        }
        
        .input-group {
            margin-bottom: 1rem;
        }
        
        .input-group label {
            display: block;
            text-align: left;
            margin-bottom: 0.5rem;
            color: #555;
            font-weight: 500;
        }
        
        .input-group input {
            width: 100%;
            padding: 12px;
            border: 2px solid #e1e5e9;
            border-radius: 6px;
            font-size: 16px;
            transition: border-color 0.2s ease;
        }
        
        .input-group input:focus {
            outline: none;
            border-color: #667eea;
            box-shadow: 0 0 0 3px rgba(102, 126, 234, 0.1);
        }
        
        .input-group input:invalid {
            border-color: #dc3545;
        }
        
        .input-hint {
            font-size: 0.85rem;
            color: #888;
            margin-top: 0.25rem;
            text-align: left;
        }
        
        .name-display {
            background: #f8f9fa;
            padding: 0.75rem;
            border-radius: 6px;
            margin-bottom: 1rem;
            border: 1px solid #e1e5e9;
        }
        
        .name-display strong {
            color: #667eea;
        }
        
        .change-name-btn {
            background: transparent;
            color: #667eea;
            border: 1px solid #667eea;
            padding: 6px 12px;
            font-size: 14px;
            margin-top: 0.5rem;
        }
        
        .change-name-btn:hover {
            background: #667eea;
            color: white;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>🎥 VideoP2P</h1>
        <p>Videollamadas directas sin apps ni registros</p>
        
        <!-- Formulario para ingresar nombre -->
        <div id="nameForm" class="name-form">
            <div class="input-group">
                <label for="userName">Tu nombre:</label>
                <input 
                    type="text" 
                    id="userName" 
                    placeholder="Ej: Juan, María, Alex..."
                    maxlength="20"
                    pattern="[a-zA-Z0-9\s]{1,20}"
                    title="Solo letras, números y espacios (máximo 20 caracteres)"
                >
                <div class="input-hint">Solo letras, números y espacios (máximo 20 caracteres)</div>
            </div>
            <button id="setNameBtn" onclick="setUserName()" disabled>Confirmar Nombre</button>
        </div>
        
        <!-- Display del nombre confirmado -->
        <div id="nameDisplay" class="name-display" style="display: none;">
            <div>Te unirás como: <strong id="displayName"></strong></div>
            <button class="change-name-btn" onclick="changeName()">Cambiar nombre</button>
        </div>
        
        <button id="createRoomBtn" onclick="createRoom()" style="display: none;">
            Crear Nueva Videollamada
        </button>
        
        <div id="linkContainer" class="link-container">
            <p><strong>¡Sala creada!</strong></p>
            <p>Comparte este enlace:</p>
            <div id="roomLink" class="room-link"></div>
            <button class="copy-btn" onclick="copyLink()">Copiar Enlace</button>
        </div>
    </div>

    <script>
        let currentRoomUrl = '';
        let currentUserName = '';

        // === MANEJO DEL NOMBRE DE USUARIO ===
        
        // Validación en tiempo real del input
        document.getElementById('userName').addEventListener('input', function(e) {
            const input = e.target;
            const setNameBtn = document.getElementById('setNameBtn');
            
            // Filtrar caracteres no permitidos en tiempo real
            const filtered = input.value.replace(/[^a-zA-Z0-9\s]/g, '');
            if (input.value !== filtered) {
                input.value = filtered;
            }
            
            // Verificar si es válido para habilitar botón
            const isValid = filtered.trim().length >= 1 && filtered.trim().length <= 20;
            setNameBtn.disabled = !isValid;
            
            // Visual feedback
            if (filtered.trim().length > 0) {
                input.style.borderColor = isValid ? '#28a745' : '#dc3545';
            } else {
                input.style.borderColor = '#e1e5e9';
            }
        });
        
        // Confirmar nombre al presionar Enter
        document.getElementById('userName').addEventListener('keypress', function(e) {
            if (e.key === 'Enter' && !document.getElementById('setNameBtn').disabled) {
                setUserName();
            }
        });
        
        function setUserName() {
            const input = document.getElementById('userName');
            const name = input.value.trim();
            
            if (name.length < 1 || name.length > 20) {
                alert('El nombre debe tener entre 1 y 20 caracteres');
                return;
            }
            
            currentUserName = name;
            
            // Ocultar formulario y mostrar display + botón de crear sala
            document.getElementById('nameForm').style.display = 'none';
            document.getElementById('nameDisplay').style.display = 'block';
            document.getElementById('displayName').textContent = currentUserName;
            document.getElementById('createRoomBtn').style.display = 'block';
            
            // Focus automático en el botón de crear sala
            document.getElementById('createRoomBtn').focus();
        }
        
        function changeName() {
            // Volver al formulario de nombre
            document.getElementById('nameForm').style.display = 'block';
            document.getElementById('nameDisplay').style.display = 'none';
            document.getElementById('createRoomBtn').style.display = 'none';
            document.getElementById('linkContainer').style.display = 'none';
            
            // Focus en el input
            document.getElementById('userName').focus();
            document.getElementById('userName').select();
        }

        async function createRoom() {
            if (!currentUserName) {
                alert('Primero debes confirmar tu nombre');
                return;
            }
            
            try {
                const response = await fetch('/api/room', { method: 'POST' });
                const room = await response.json();
                
                // Agregar el nombre como parámetro en la URL
                currentRoomUrl = window.location.origin + room.url + '?name=' + encodeURIComponent(currentUserName);
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
        
        /* === MODAL PARA NOMBRE === */
        .name-modal {
            position: fixed;
            top: 0; left: 0; right: 0; bottom: 0;
            background: rgba(0, 0, 0, 0.8);
            display: flex;
            align-items: center;
            justify-content: center;
            z-index: 1000;
        }
        
        .name-modal-content {
            background: white;
            padding: 2rem;
            border-radius: 12px;
            max-width: 400px;
            width: 90%;
            text-align: center;
        }
        
        .name-modal h2 {
            color: #333;
            margin-bottom: 1rem;
        }
        
        .name-modal p {
            color: #666;
            margin-bottom: 1.5rem;
        }
        
        .name-modal input {
            width: 100%;
            padding: 12px;
            border: 2px solid #e1e5e9;
            border-radius: 6px;
            font-size: 16px;
            margin-bottom: 1rem;
            transition: border-color 0.2s ease;
        }
        
        .name-modal input:focus {
            outline: none;
            border-color: #667eea;
            box-shadow: 0 0 0 3px rgba(102, 126, 234, 0.1);
        }
        
        .name-modal button {
            background: #667eea;
            color: white;
            border: none;
            padding: 12px 24px;
            border-radius: 6px;
            font-size: 16px;
            cursor: pointer;
            width: 100%;
            margin-bottom: 0.5rem;
            transition: background 0.2s;
        }
        
        .name-modal button:hover { background: #5a6fd8; }
        .name-modal button:disabled { 
            background: #ccc; 
            cursor: not-allowed; 
        }
        
        .name-modal .hint {
            font-size: 0.85rem;
            color: #888;
            margin-top: 0.5rem;
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
            display: flex;
            align-items: center;
            justify-content: center;
        }
        
        .video-wrapper.speaking {
            border-color: #28a745;
            box-shadow: 0 0 15px rgba(40, 167, 69, 0.5);
        }
        
        .video-wrapper.local {
            border-color: #667eea;
        }

        .video-wrapper.connecting {
            border-color: #ffc107;
        }

        /* === NUEVOS ESTILOS PARA CONTROLES === */
        .video-wrapper.cam-off video {
            display: none;
        }

        .video-wrapper.cam-off::after {
            content: '📷';
            font-size: 4rem;
            color: rgba(255, 255, 255, 0.7);
            display: flex;
            align-items: center;
            justify-content: center;
            position: absolute;
            top: 0; left: 0; right: 0; bottom: 0;
            background: rgba(0, 0, 0, 0.8);
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

        /* === ESTILOS PARA SPINNERS === */
        .connection-spinner {
            display: flex;
            flex-direction: column;
            align-items: center;
            gap: 15px;
        }

        .spinner-circle {
            width: 50px;
            height: 50px;
            border: 4px solid rgba(255, 255, 255, 0.3);
            border-top: 4px solid white;
            border-radius: 50%%;
            animation: spin 1s linear infinite;
        }

        .spinner-text {
            font-size: 1rem;
            color: rgba(255, 255, 255, 0.8);
        }

        @keyframes spin {
            0%% { transform: rotate(0deg); }
            100%% { transform: rotate(360deg); }
        }
        
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
            font-size: 1.2rem;
        }
        .control-btn:hover { 
            background: #555; 
            transform: scale(1.05);
        }

        /* === NUEVOS ESTILOS PARA BOTONES ACTIVOS === */
        .control-btn.active { 
            background: #dc3545 !important;
            animation: pulse 1.5s infinite;
        }

        @keyframes pulse {
            0%%, 100%% { opacity: 1; }
            50%% { opacity: 0.7; }
        }

        .status {
            color: white;
            text-align: center;
            padding: 1rem;
        }
        .connecting { color: #ffc107; }
        .connected { color: #28a745; }
        .error { color: #dc3545; }

        /* === INDICADOR DE TECLAS DE ACCESO RÁPIDO === */
        .shortcuts-hint {
            position: fixed;
            top: 20px;
            right: 20px;
            background: rgba(0, 0, 0, 0.8);
            color: white;
            padding: 10px 15px;
            border-radius: 8px;
            font-size: 0.8rem;
            backdrop-filter: blur(10px);
            opacity: 0.7;
            transition: opacity 0.3s ease;
        }

        .shortcuts-hint:hover {
            opacity: 1;
        }
    </style>
</head>
<body>
    <!-- Modal para ingresar nombre -->
    <div id="nameModal" class="name-modal">
        <div class="name-modal-content">
            <h2>🎥 Únete a la videollamada</h2>
            <p>¿Cómo te quieres llamar en esta sala?</p>
            <input 
                type="text" 
                id="modalUserName" 
                placeholder="Tu nombre..."
                maxlength="20"
                pattern="[a-zA-Z0-9\s]{1,20}"
                title="Solo letras, números y espacios"
            >
            <button id="joinRoomBtn" onclick="joinWithName()" disabled>
                Unirse a la Sala
            </button>
            <div class="hint">Solo letras, números y espacios (máximo 20 caracteres)</div>
        </div>
    </div>
    
    <div class="header">
        <h2>Sala: %s</h2>
        <div id="status" class="status connecting">Conectando...</div>
    </div>
    
    <div class="video-container" id="videoContainer">
        <!-- Los videos se agregarán dinámicamente aquí -->
    </div>
    
    <div class="controls">
        <button id="micBtn" class="control-btn" onclick="toggleMic()" title="Alternar micrófono (M)">🎤</button>
        <button id="camBtn" class="control-btn" onclick="toggleCam()" title="Alternar cámara (V)">📹</button>
        <button id="hangupBtn" class="control-btn" onclick="hangup()" title="Colgar llamada (H)">📞</button>
    </div>

    <!-- Indicador de teclas de acceso rápido -->
    <div class="shortcuts-hint">
        💡 Teclas: M (mic), V (video), H (colgar)
    </div>

    <script src="/static/webrtc.js"></script>
    <script>
        const roomId = '%s';
        let videoCall = null;
        
        // === MANEJO DEL NOMBRE AL ENTRAR A LA SALA ===
        
        function getUserNameFromURL() {
            const urlParams = new URLSearchParams(window.location.search);
            return urlParams.get('name') || '';
        }
        
        function setupNameModal() {
            const nameFromUrl = getUserNameFromURL();
            const input = document.getElementById('modalUserName');
            const joinBtn = document.getElementById('joinRoomBtn');
            
            // Si viene nombre en URL, pre-llenarlo
            if (nameFromUrl) {
                input.value = nameFromUrl;
                // Trigger validation
                input.dispatchEvent(new Event('input'));
            }
            
            // Validación en tiempo real
            input.addEventListener('input', function(e) {
                const filtered = e.target.value.replace(/[^a-zA-Z0-9\s]/g, '');
                if (e.target.value !== filtered) {
                    e.target.value = filtered;
                }
                
                const isValid = filtered.trim().length >= 1 && filtered.trim().length <= 20;
                joinBtn.disabled = !isValid;
                
                if (filtered.trim().length > 0) {
                    e.target.style.borderColor = isValid ? '#28a745' : '#dc3545';
                } else {
                    e.target.style.borderColor = '#e1e5e9';
                }
            });
            
            // Enter para unirse
            input.addEventListener('keypress', function(e) {
                if (e.key === 'Enter' && !joinBtn.disabled) {
                    joinWithName();
                }
            });
            
            // Focus automático
            input.focus();
            if (nameFromUrl) {
                input.select();
            }
        }
        
        function joinWithName() {
            const input = document.getElementById('modalUserName');
            const name = input.value.trim();
            
            if (name.length < 1 || name.length > 20) {
                alert('El nombre debe tener entre 1 y 20 caracteres');
                return;
            }
            
            // Ocultar modal
            document.getElementById('nameModal').style.display = 'none';
            
            // Inicializar VideoCall con el nombre
            videoCall = new VideoCall(roomId, name);
            window.videoCall = videoCall;
            videoCall.init();
        }
        
        // === INICIALIZACIÓN ===
        document.addEventListener('DOMContentLoaded', function() {
            setupNameModal();
        });
    </script>
</body>
</html>`, roomId, roomId, roomId, roomId, roomId, roomId)
}
