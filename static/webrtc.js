// webrtc.js - Compatible con tu signaling server
class VideoCall {
    constructor(roomId) {
        this.roomId = roomId;
        this.ws = null;
        this.pc = null;
        this.localStream = null;
        this.remoteStream = null;
        this.myPeerId = null;
        this.remotePeerId = null;

        this.config = {
            iceServers: [
                { urls: 'stun:stun.l.google.com:19302' },
                { urls: 'stun:stun1.l.google.com:19302' }
            ]
        };

        this.isConnected = false;
        this.isMicOn = true;
        this.isCamOn = true;
        this.debug = true;
    }

    log(...args) {
        if (this.debug) {
            console.log('[VideoCall]', ...args);
        }
    }

    async init() {
        try {
            this.log('Inicializando VideoCall para sala:', this.roomId);

            // Obtener configuración del servidor
            await this.loadConfig();

            // Obtener media local
            this.updateStatus('Accediendo a cámara y micrófono...', 'connecting');
            await this.setupLocalMedia();

            // Conectar al signaling server
            this.updateStatus('Conectando al servidor...', 'connecting');
            await this.connectSignaling();

            // Setup WebRTC
            this.setupWebRTC();

            this.updateStatus('Esperando al otro participante...', 'connecting');
            this.log('Inicialización completa');

        } catch (error) {
            console.error('Error inicializando:', error);
            this.updateStatus('Error: ' + error.message, 'error');
        }
    }

    async loadConfig() {
        try {
            this.log('Cargando configuración...');
            const response = await fetch('/api/config');

            if (!response.ok) {
                throw new Error('HTTP ' + response.status + ': ' + response.statusText);
            }

            const config = await response.json();
            this.log('Configuración recibida:', config);

            this.signalingUrl = config.signalingUrl;

            if (config.stunServers) {
                this.config.iceServers = config.stunServers.map(url => ({ urls: url }));
            }

        } catch (error) {
            console.warn('No se pudo cargar configuración, usando defaults:', error);
            this.signalingUrl = 'ws://localhost:8081/room';
        }

        this.log('Signaling URL configurada:', this.signalingUrl);
    }

    async setupLocalMedia() {
        try {
            this.log('Solicitando acceso a media...');

            this.localStream = await navigator.mediaDevices.getUserMedia({
                video: {
                    width: { ideal: 1280 },
                    height: { ideal: 720 },
                    facingMode: 'user'
                },
                audio: {
                    echoCancellation: true,
                    noiseSuppression: true,
                    autoGainControl: true
                }
            });

            this.log('Media local obtenida:', {
                videoTracks: this.localStream.getVideoTracks().length,
                audioTracks: this.localStream.getAudioTracks().length
            });

            const localVideo = document.getElementById('localVideo');
            localVideo.srcObject = this.localStream;

        } catch (error) {
            throw new Error('No se pudo acceder a cámara/micrófono: ' + error.message);
        }
    }

    connectSignaling() {
        return new Promise((resolve, reject) => {
            // Tu signaling server espera: ws://localhost:8081/room/{roomId}
            const wsUrl = `${this.signalingUrl}/${this.roomId}`;
            this.log('Conectando a WebSocket:', wsUrl);

            try {
                this.ws = new WebSocket(wsUrl);
            } catch (error) {
                this.log('Error creando WebSocket:', error);
                reject(new Error('Error creando conexión WebSocket: ' + error.message));
                return;
            }

            const timeout = setTimeout(() => {
                this.log('Timeout de conexión WebSocket');
                if (this.ws.readyState !== WebSocket.OPEN) {
                    this.ws.close();
                    reject(new Error('Timeout conectando al signaling server. ¿Está corriendo en ' + this.signalingUrl + '?'));
                }
            }, 10000);

            this.ws.onopen = () => {
                this.log('WebSocket conectado exitosamente');
                clearTimeout(timeout);
                resolve();
            };

            this.ws.onmessage = (event) => {
                try {
                    const message = JSON.parse(event.data);
                    this.log('Mensaje recibido:', message);
                    this.handleSignalingMessage(message);
                } catch (error) {
                    console.error('Error parseando mensaje:', error);
                }
            };

            this.ws.onclose = (event) => {
                this.log('WebSocket cerrado:', event.code, event.reason);
                clearTimeout(timeout);

                if (event.code !== 1000) {
                    this.updateStatus('Conexión perdida con el servidor', 'error');
                }
            };

            this.ws.onerror = (error) => {
                this.log('Error en WebSocket:', error);
                clearTimeout(timeout);
                reject(new Error('Error de conexión WebSocket. Verifica que el signaling server esté corriendo.'));
            };
        });
    }

    setupWebRTC() {
        this.log('Configurando WebRTC...');
        this.pc = new RTCPeerConnection(this.config);

        // Agregar tracks locales
        this.localStream.getTracks().forEach(track => {
            this.log('Agregando track:', track.kind);
            this.pc.addTrack(track, this.localStream);
        });

        // Manejar stream remoto
        this.pc.ontrack = (event) => {
            this.log('Stream remoto recibido:', event.streams.length, 'streams');
            const remoteVideo = document.getElementById('remoteVideo');
            remoteVideo.srcObject = event.streams[0];
            this.updateStatus('¡Conectado!', 'connected');
            this.isConnected = true;
        };

        // Manejar ICE candidates
        this.pc.onicecandidate = (event) => {
            if (event.candidate) {
                this.log('ICE candidate:', event.candidate.candidate);
                this.sendSignalingMessage({
                    type: 'ice_candidate',
                    data: event.candidate,
                    target: this.remotePeerId
                });
            } else {
                this.log('ICE gathering completado');
            }
        };

        this.pc.onconnectionstatechange = () => {
            this.log('Estado de conexión WebRTC:', this.pc.connectionState);

            switch (this.pc.connectionState) {
                case 'connected':
                    this.updateStatus('¡Conectado!', 'connected');
                    this.isConnected = true;
                    break;
                case 'disconnected':
                case 'failed':
                    this.updateStatus('Conexión P2P perdida', 'error');
                    this.isConnected = false;
                    break;
                case 'connecting':
                    this.updateStatus('Estableciendo conexión P2P...', 'connecting');
                    break;
            }
        };

        this.log('WebRTC configurado');
    }

    async handleSignalingMessage(message) {
        try {
            this.log('Manejando mensaje:', message.type);

            switch (message.type) {
                case 'peer_joined':
                    this.log('Peer se unió:', message.peer_id);
                    this.remotePeerId = message.peer_id;
                    this.updateStatus('Otro usuario se unió. Iniciando conexión...', 'connecting');

                    // Si es el primer peer que se une, crear offer
                    await this.createOffer();
                    break;

                case 'peer_left':
                    this.log('Peer se fue:', message.peer_id);
                    this.handleUserLeft();
                    break;

                case 'offer':
                    await this.handleOffer(message);
                    break;

                case 'answer':
                    await this.handleAnswer(message);
                    break;

                case 'ice_candidate':
                    await this.handleIceCandidate(message);
                    break;

                default:
                    this.log('Mensaje desconocido:', message);
            }
        } catch (error) {
            console.error('Error manejando mensaje de signaling:', error);
        }
    }

    async createOffer() {
        try {
            this.log('Creando offer...');
            const offer = await this.pc.createOffer();
            await this.pc.setLocalDescription(offer);

            this.log('Offer creado y configurado localmente');

            this.sendSignalingMessage({
                type: 'offer',
                data: offer,
                target: this.remotePeerId
            });
        } catch (error) {
            console.error('Error creando offer:', error);
        }
    }

    async handleOffer(message) {
        try {
            this.log('Manejando offer recibido');
            this.remotePeerId = message.peer_id;

            await this.pc.setRemoteDescription(message.data);

            const answer = await this.pc.createAnswer();
            await this.pc.setLocalDescription(answer);

            this.log('Answer creado y enviado');

            this.sendSignalingMessage({
                type: 'answer',
                data: answer,
                target: message.peer_id
            });
        } catch (error) {
            console.error('Error manejando offer:', error);
        }
    }

    async handleAnswer(message) {
        try {
            this.log('Manejando answer recibido');
            await this.pc.setRemoteDescription(message.data);
        } catch (error) {
            console.error('Error manejando answer:', error);
        }
    }

    async handleIceCandidate(message) {
        try {
            this.log('Agregando ICE candidate');
            await this.pc.addIceCandidate(message.data);
        } catch (error) {
            console.error('Error agregando ICE candidate:', error);
        }
    }

    handleUserLeft() {
        this.log('Usuario remoto se desconectó');
        this.updateStatus('El otro usuario se desconectó', 'error');
        const remoteVideo = document.getElementById('remoteVideo');
        remoteVideo.srcObject = null;
        this.remotePeerId = null;
        this.isConnected = false;
    }

    sendSignalingMessage(message) {
        if (this.ws && this.ws.readyState === WebSocket.OPEN) {
            this.log('Enviando mensaje:', message);
            this.ws.send(JSON.stringify(message));
        } else {
            this.log('WebSocket no está conectado, no se puede enviar:', message);
        }
    }

    updateStatus(message, type) {
        const statusEl = document.getElementById('status');
        statusEl.textContent = message;
        statusEl.className = `status ${type}`;
        this.log('Status actualizado:', message, `(${type})`);
    }

    toggleMic() {
        this.isMicOn = !this.isMicOn;

        const audioTrack = this.localStream.getAudioTracks()[0];
        if (audioTrack) {
            audioTrack.enabled = this.isMicOn;
        }

        const micBtn = document.getElementById('micBtn');
        micBtn.textContent = this.isMicOn ? '🎤' : '🔇';
        micBtn.classList.toggle('active', !this.isMicOn);

        this.log('Micrófono:', this.isMicOn ? 'activado' : 'desactivado');
    }

    toggleCam() {
        this.isCamOn = !this.isCamOn;

        const videoTrack = this.localStream.getVideoTracks()[0];
        if (videoTrack) {
            videoTrack.enabled = this.isCamOn;
        }

        const camBtn = document.getElementById('camBtn');
        camBtn.textContent = this.isCamOn ? '📹' : '📷';
        camBtn.classList.toggle('active', !this.isCamOn);

        this.log('Cámara:', this.isCamOn ? 'activada' : 'desactivada');
    }

    hangup() {
        this.log('Colgando llamada...');

        if (this.pc) {
            this.pc.close();
        }

        if (this.ws) {
            this.ws.close();
        }

        if (this.localStream) {
            this.localStream.getTracks().forEach(track => track.stop());
        }

        window.location.href = '/';
    }
}

// Funciones globales para los botones
function toggleMic() {
    if (window.videoCall) {
        window.videoCall.toggleMic();
    }
}

function toggleCam() {
    if (window.videoCall) {
        window.videoCall.toggleCam();
    }
}

function hangup() {
    if (window.videoCall) {
        window.videoCall.hangup();
    }
}

// Exponer la instancia globalmente
window.VideoCall = VideoCall;