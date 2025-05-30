// webrtc.js - Multi-participant con spinner mejorado
class VideoCall {
    constructor(roomId) {
        this.roomId = roomId;
        this.ws = null;
        this.peers = new Map(); // Map de peerID -> { pc, stream, isLocal, retryCount, connectionTimer }
        this.localStream = null;
        this.myPeerId = null;
        this.serverParticipantCount = 1; // Conteo autoritativo del servidor

        // Lista mejorada de STUN servers con fallback automático
        this.stunServerTiers = [
            // Tier 1 - Más confiables
            [
                { urls: 'stun:openrelay.metered.ca:80' },
                { urls: 'stun:stunserver2024.stunprotocol.org:3478' },
                { urls: 'stun:stun.l.google.com:19302' },
                { urls: 'stun:stun1.l.google.com:19302' }
            ],
            // Tier 2 - Confiables con buena distribución
            [
                { urls: 'stun:stun.cloudflare.com:3478' },
                { urls: 'stun:stun.mozilla.org:3478' },
                { urls: 'stun:stun.nextcloud.com:443' },
                { urls: 'stun:stun.3cx.com:3478' }
            ],
            // Tier 3 - Alternativas robustas
            [
                { urls: 'stun:stun.antisip.com:3478' },
                { urls: 'stun:stun.voipbuster.com:3478' }
            ]
        ];

        this.currentStunTier = 0; // Empezar con Tier 1
        this.config = {
            iceServers: this.stunServerTiers[0]
        };

        // Configuración de retry
        this.maxRetries = 3;
        this.connectionTimeout = 15000; // 15 segundos
        this.retryDelay = 2000; // 2 segundos entre reintentos

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

            await this.loadConfig();

            this.updateStatus('Accediendo a cámara y micrófono...', 'connecting');
            await this.setupLocalMedia();

            this.updateStatus('Conectando al servidor...', 'connecting');
            await this.connectSignaling();

            this.updateStatus('Esperando a otros participantes...', 'connecting');
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

            // Agregar nuestro video local al UI
            this.addVideoToUI('local', this.localStream, 'Tú (Local)', true);

        } catch (error) {
            throw new Error('No se pudo acceder a cámara/micrófono: ' + error.message);
        }
    }

    connectSignaling() {
        return new Promise((resolve, reject) => {
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
                    reject(new Error('Timeout conectando al signaling server'));
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
                reject(new Error('Error de conexión WebSocket'));
            };
        });
    }

    async handleSignalingMessage(message) {
        try {
            this.log('Manejando mensaje:', message.type, 'de peer:', message.peer_id);

            switch (message.type) {
                case 'joined':
                    // Cuando nos unimos a la sala, recibimos nuestro peer ID y conteo del servidor
                    this.myPeerId = message.peer_id;
                    if (message.data && message.data.participant_count) {
                        this.serverParticipantCount = message.data.participant_count;
                    }
                    this.log('Mi peer ID:', this.myPeerId, 'Participantes en servidor:', this.serverParticipantCount);
                    this.updateParticipantCount();
                    break;

                case 'peer_joined':
                    // Actualizar conteo del servidor si viene en el mensaje
                    if (message.data && message.data.participant_count) {
                        this.serverParticipantCount = message.data.participant_count;
                        this.log('Conteo actualizado del servidor:', this.serverParticipantCount);
                    }
                    await this.handlePeerJoined(message.peer_id);
                    break;

                case 'peer_left':
                    // Actualizar conteo del servidor si viene en el mensaje
                    if (message.data && message.data.participant_count) {
                        this.serverParticipantCount = message.data.participant_count;
                        this.log('Conteo actualizado del servidor (peer left):', this.serverParticipantCount);
                    }
                    this.handlePeerLeft(message.peer_id);
                    break;

                case 'user_media_changed':
                    // Actualizar estado de media de otros usuarios
                    this.handleUserMediaChanged(message);
                    break;

                case 'connection_quality_warning':
                    // Manejar advertencias de calidad de conexión
                    this.handleQualityWarning(message);
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

    handleUserMediaChanged(message) {
        const { peerId, micOn, camOn } = message.data;

        // No procesar nuestros propios cambios
        if (peerId === this.myPeerId) {
            return;
        }

        this.log(`📺 ${peerId} cambió estado: mic=${micOn}, cam=${camOn}`);

        // Actualizar iconos de estado visual
        this.updateVideoStatus(peerId, micOn, camOn);
    }

    handleQualityWarning(message) {
        const { reason, suggestion, from } = message.data;

        this.log(`⚠️ Advertencia de calidad de ${from}: ${reason}, sugerencia: ${suggestion}`);

        // Ejemplo: reducir calidad automáticamente si es sugerido
        if (suggestion === 'reduce_quality') {
            this.log('🔧 Reduciendo calidad de video automáticamente');
            // Aquí podrías implementar lógica para reducir bitrate, resolución, etc.
        }
    }

    async handlePeerJoined(peerId) {
        this.log('Peer se unió:', peerId, 'Total peers antes:', this.peers.size);

        // Verificar que no es el mismo peer ID que ya tenemos
        if (this.peers.has(peerId)) {
            this.log('Peer ya existe, ignorando:', peerId);
            return;
        }

        // Verificar que no es nuestro propio peer ID
        if (peerId === this.myPeerId) {
            this.log('Ignorando nuestro propio peer ID:', peerId);
            return;
        }

        // Agregar spinner inmediatamente para mostrar que el peer está conectándose
        this.addSpinnerForPeer(peerId, `Participante ${peerId.slice(-4)}`);

        // Crear conexión peer para el nuevo participante (con retry automático)
        const pc = await this.createPeerConnection(peerId, 0);

        // Agregar nuestro stream local a la conexión
        this.localStream.getTracks().forEach(track => {
            pc.addTrack(track, this.localStream);
        });

        // Crear offer
        this.log('Creando offer para:', peerId);
        const offer = await pc.createOffer();
        await pc.setLocalDescription(offer);

        this.sendSignalingMessage({
            type: 'offer',
            data: offer,
            target: peerId
        });

        this.log('Peers después de agregar:', this.peers.size);
        this.updateParticipantCount();
    }

    handlePeerLeft(peerId) {
        this.log('Peer se fue:', peerId);

        // Cerrar conexión y limpiar timers
        if (this.peers.has(peerId)) {
            const peer = this.peers.get(peerId);
            if (peer.pc) {
                peer.pc.close();
            }
            if (peer.connectionTimer) {
                clearTimeout(peer.connectionTimer);
            }
            this.peers.delete(peerId);
            this.log('Peer removido del Map. Total peers:', this.peers.size);
        }

        // Remover video del UI
        this.removeVideoFromUI(peerId);

        // El conteo se actualiza automáticamente con el mensaje del servidor
        this.updateParticipantCount();
    }

    async handleOffer(message) {
        this.log('Manejando offer recibido de:', message.peer_id);

        // Agregar spinner si no existe el elemento
        if (!document.getElementById(`video-${message.peer_id}`)) {
            this.addSpinnerForPeer(message.peer_id, `Participante ${message.peer_id.slice(-4)}`);
        }

        // Crear nueva conexión si no existe
        let pc;
        if (this.peers.has(message.peer_id)) {
            const peer = this.peers.get(message.peer_id);
            pc = peer.pc;
            this.log('Usando conexión existente para:', message.peer_id);
        } else {
            pc = await this.createPeerConnection(message.peer_id, 0);

            // Agregar nuestro stream local
            this.localStream.getTracks().forEach(track => {
                pc.addTrack(track, this.localStream);
            });
        }

        try {
            await pc.setRemoteDescription(message.data);

            const answer = await pc.createAnswer();
            await pc.setLocalDescription(answer);

            this.sendSignalingMessage({
                type: 'answer',
                data: answer,
                target: message.peer_id
            });

            this.updateParticipantCount();

        } catch (error) {
            this.log(`❌ Error procesando offer de ${message.peer_id}:`, error);

            // Si falla el offer, intentar recrear la conexión
            if (this.peers.has(message.peer_id)) {
                const peer = this.peers.get(message.peer_id);
                if (peer.retryCount < this.maxRetries) {
                    this.log(`🔄 Reintentando conexión con ${message.peer_id} después de error en offer`);
                    setTimeout(() => {
                        this.retryPeerConnection(message.peer_id, peer.retryCount + 1);
                    }, this.retryDelay);
                }
            }
        }
    }

    async handleAnswer(message) {
        this.log('Manejando answer recibido de:', message.peer_id);

        const peer = this.peers.get(message.peer_id);
        if (peer && peer.pc) {
            await peer.pc.setRemoteDescription(message.data);
        }
    }

    async handleIceCandidate(message) {
        this.log('Agregando ICE candidate de:', message.peer_id);

        const peer = this.peers.get(message.peer_id);
        if (peer && peer.pc) {
            await peer.pc.addIceCandidate(message.data);
        }
    }

    async createPeerConnection(peerId, retryCount = 0) {
        this.log(`Creando conexión peer para: ${peerId} (intento ${retryCount + 1}/${this.maxRetries + 1})`);

        // Seleccionar STUN servers basado en el número de reintentos
        const stunTierIndex = Math.min(retryCount, this.stunServerTiers.length - 1);
        const currentStunServers = this.stunServerTiers[stunTierIndex];

        const config = {
            iceServers: currentStunServers
        };

        this.log(`Usando STUN Tier ${stunTierIndex + 1}:`, currentStunServers.map(s => s.urls));

        const pc = new RTCPeerConnection(config);

        // Variables para tracking de conexión
        let connectionTimer = null;
        let isConnectionEstablished = false;

        // Manejar stream remoto
        pc.ontrack = (event) => {
            this.log('Stream remoto recibido de:', peerId);
            const remoteStream = event.streams[0];

            // Marcar conexión como exitosa
            isConnectionEstablished = true;
            if (connectionTimer) {
                clearTimeout(connectionTimer);
                connectionTimer = null;
            }

            // Reemplazar spinner con video real
            this.replaceSpinnerWithVideo(peerId, remoteStream, `Participante ${peerId.slice(-4)}`);
            this.log(`✅ Conexión WebRTC exitosa con ${peerId} en intento ${retryCount + 1}`);
        };

        // Manejar ICE candidates
        pc.onicecandidate = (event) => {
            if (event.candidate) {
                this.sendSignalingMessage({
                    type: 'ice_candidate',
                    data: event.candidate,
                    target: peerId
                });
            }
        };

        // Manejar cambios de estado de conexión
        pc.onconnectionstatechange = () => {
            this.log(`Estado de conexión con ${peerId}:`, pc.connectionState);

            if (pc.connectionState === 'connected') {
                isConnectionEstablished = true;
                if (connectionTimer) {
                    clearTimeout(connectionTimer);
                    connectionTimer = null;
                }
                this.updateParticipantCount();

            } else if (pc.connectionState === 'failed' || pc.connectionState === 'disconnected') {
                this.log(`❌ Conexión con ${peerId} falló (${pc.connectionState})`);

                if (connectionTimer) {
                    clearTimeout(connectionTimer);
                    connectionTimer = null;
                }

                // Si la conexión falló y no hemos superado los reintentos, intentar de nuevo
                if (!isConnectionEstablished && retryCount < this.maxRetries) {
                    this.log(`🔄 Programando reintento para ${peerId} en ${this.retryDelay}ms`);
                    setTimeout(() => {
                        this.retryPeerConnection(peerId, retryCount + 1);
                    }, this.retryDelay);
                } else if (!isConnectionEstablished) {
                    // Si agotamos los reintentos, notificar al servidor que remueva este peer
                    this.log(`💀 Agotados los reintentos para ${peerId}, notificando al servidor`);
                    this.notifyServerPeerConnectionFailed(peerId);
                }

                // Limpiar la conexión fallida
                this.handlePeerLeft(peerId);
            }
        };

        // Configurar timeout para la conexión
        connectionTimer = setTimeout(() => {
            if (!isConnectionEstablished) {
                this.log(`⏰ Timeout de conexión para ${peerId} después de ${this.connectionTimeout}ms`);

                if (retryCount < this.maxRetries) {
                    this.log(`🔄 Reintentando conexión con ${peerId} (timeout)`);
                    pc.close();
                    this.retryPeerConnection(peerId, retryCount + 1);
                } else {
                    this.log(`💀 Timeout final para ${peerId}, notificando al servidor`);
                    pc.close();
                    this.notifyServerPeerConnectionFailed(peerId);
                    this.handlePeerLeft(peerId);
                }
            }
        }, this.connectionTimeout);

        // Guardar la conexión con información de retry
        this.peers.set(peerId, {
            pc,
            stream: null,
            isLocal: false,
            retryCount: retryCount,
            connectionTimer: connectionTimer
        });

        return pc;
    }

    addSpinnerForPeer(peerId, label) {
        // No agregar spinner si ya existe el video
        if (document.getElementById(`video-${peerId}`)) {
            return;
        }

        const container = document.getElementById('videoContainer');

        // Crear wrapper del spinner
        const wrapper = document.createElement('div');
        wrapper.className = 'video-wrapper connecting';
        wrapper.id = `video-${peerId}`;

        // Crear spinner
        const spinner = document.createElement('div');
        spinner.className = 'connection-spinner';
        spinner.innerHTML = `
            <div class="spinner-circle"></div>
            <div class="spinner-text">Conectando...</div>
        `;

        // Crear label
        const labelEl = document.createElement('div');
        labelEl.className = 'video-label';
        labelEl.textContent = label;

        wrapper.appendChild(spinner);
        wrapper.appendChild(labelEl);

        container.appendChild(wrapper);
        this.updateLayout();

        this.log(`Spinner agregado para ${peerId} (${label})`);
    }

    removeSpinner(peerId) {
        const wrapper = document.getElementById(`video-${peerId}`);
        if (wrapper) {
            const spinner = wrapper.querySelector('.connection-spinner');
            if (spinner) {
                spinner.remove();
                wrapper.classList.remove('connecting');
                this.log(`Spinner removido para ${peerId}`);
            }
        }
    }

    replaceSpinnerWithVideo(peerId, stream, label) {
        const wrapper = document.getElementById(`video-${peerId}`);
        if (!wrapper) {
            // Si no hay wrapper, crear video normalmente
            this.addVideoToUI(peerId, stream, label);
            return;
        }

        // Remover spinner
        const spinner = wrapper.querySelector('.connection-spinner');
        if (spinner) {
            spinner.remove();
        }

        // Agregar video
        const video = document.createElement('video');
        video.autoplay = true;
        video.playsInline = true;
        video.muted = false;
        video.srcObject = stream;

        // Crear status icons si no existen
        if (!wrapper.querySelector('.video-status')) {
            const statusEl = document.createElement('div');
            statusEl.className = 'video-status';
            statusEl.id = `status-${peerId}`;
            wrapper.appendChild(statusEl);
        }

        // Insertar video al principio del wrapper
        wrapper.insertBefore(video, wrapper.firstChild);
        wrapper.classList.remove('connecting');

        this.log(`Spinner reemplazado con video para ${peerId}`);
    }

    addVideoToUI(peerId, stream, label, isLocal = false) {
        // Verificar que no existe ya
        if (document.getElementById(`video-${peerId}`)) {
            this.log(`Video para ${peerId} ya existe, actualizando stream`);
            const existingVideo = document.querySelector(`#video-${peerId} video`);
            if (existingVideo) {
                existingVideo.srcObject = stream;
                // Remover spinner si existe
                this.removeSpinner(peerId);
            }
            return;
        }

        const container = document.getElementById('videoContainer');

        // Crear wrapper del video
        const wrapper = document.createElement('div');
        wrapper.className = `video-wrapper ${isLocal ? 'local' : ''}`;
        wrapper.id = `video-${peerId}`;

        // Crear elemento video
        const video = document.createElement('video');
        video.autoplay = true;
        video.playsInline = true;
        video.muted = isLocal; // Solo mutear nuestro propio audio
        video.srcObject = stream;

        // Crear label
        const labelEl = document.createElement('div');
        labelEl.className = 'video-label';
        labelEl.textContent = label;

        // Crear status icons
        const statusEl = document.createElement('div');
        statusEl.className = 'video-status';
        statusEl.id = `status-${peerId}`;

        wrapper.appendChild(video);
        wrapper.appendChild(labelEl);
        wrapper.appendChild(statusEl);

        container.appendChild(wrapper);

        this.updateLayout();

        this.log(`Video agregado para ${peerId} (${label})`);
    }

    removeVideoFromUI(peerId) {
        const videoElement = document.getElementById(`video-${peerId}`);
        if (videoElement) {
            videoElement.remove();
            this.updateLayout();
            this.log(`Video removido para ${peerId}`);
        }
    }

    async retryPeerConnection(peerId, retryCount) {
        this.log(`🔄 Reintentando conexión con ${peerId} (intento ${retryCount + 1}/${this.maxRetries + 1})`);

        // Limpiar conexión anterior si existe
        if (this.peers.has(peerId)) {
            const peer = this.peers.get(peerId);
            if (peer.pc) {
                peer.pc.close();
            }
            if (peer.connectionTimer) {
                clearTimeout(peer.connectionTimer);
            }
            this.peers.delete(peerId);
        }

        // Remover video anterior si existe
        this.removeVideoFromUI(peerId);

        // Agregar nuevo spinner para el reintento
        this.addSpinnerForPeer(peerId, `Participante ${peerId.slice(-4)}`);

        // Crear nueva conexión con retry count
        const pc = await this.createPeerConnection(peerId, retryCount);

        // Agregar nuestro stream local
        this.localStream.getTracks().forEach(track => {
            pc.addTrack(track, this.localStream);
        });

        // Crear nueva offer
        const offer = await pc.createOffer();
        await pc.setLocalDescription(offer);

        this.sendSignalingMessage({
            type: 'offer',
            data: offer,
            target: peerId
        });
    }

    notifyServerPeerConnectionFailed(peerId) {
        this.log(`📤 Notificando al servidor que la conexión con ${peerId} falló`);

        // Enviar mensaje al servidor para que remueva este peer del conteo
        this.sendSignalingMessage({
            type: 'connection_failed',
            target: peerId,
            data: {
                reason: 'webrtc_connection_failed',
                failed_peer: peerId
            }
        });
    }

    // Ejemplo de mensaje a múltiples peers específicos
    notifySlowPeers(slowPeerIds, reason) {
        if (slowPeerIds.length === 0) return;

        this.log(`📤 Notificando a peers lentos sobre optimización:`, slowPeerIds);

        // Enviar solo a peers específicos que tienen problemas
        this.sendSignalingMessage({
            type: 'connection_quality_warning',
            data: {
                reason: reason,
                suggestion: 'reduce_quality',
                from: this.myPeerId
            },
            target: slowPeerIds  // ← Array de peer IDs específicos
        });
    }

    // Ejemplo de mensaje a todos excepto algunos
    notifyAllExcept(excludePeerIds, messageType, data) {
        // Obtener todos los peer IDs conectados
        const allPeerIds = Array.from(this.peers.keys());
        const targetPeerIds = allPeerIds.filter(peerId => !excludePeerIds.includes(peerId));

        if (targetPeerIds.length === 0) return;

        this.sendSignalingMessage({
            type: messageType,
            data: data,
            target: targetPeerIds  // ← Solo a peers específicos
        });
    }

    updateLayout() {
        const container = document.getElementById('videoContainer');
        const participantCount = container.children.length;

        // Remover clases de participantes previas
        container.className = container.className.replace(/participants-\d+/g, '');

        // Agregar nueva clase basada en el número de participantes
        container.classList.add(`participants-${Math.min(participantCount, 9)}`);

        this.log(`Layout actualizado para ${participantCount} participantes`);
    }

    updateParticipantCount() {
        // Usar el conteo del servidor como fuente de verdad
        const serverCount = this.serverParticipantCount;

        // Contar videos reales (sin spinners) y spinners por separado
        const container = document.getElementById('videoContainer');
        const totalElements = container.children.length;
        const videoElements = container.querySelectorAll('video').length;
        const spinnerElements = container.querySelectorAll('.connection-spinner').length;

        this.log(`Participantes - Servidor: ${serverCount}, Videos: ${videoElements}, Spinners: ${spinnerElements}, Total DOM: ${totalElements}`);

        // Mostrar el conteo del servidor (fuente de verdad)
        if (serverCount === 1) {
            this.updateStatus('Solo tú en la sala', 'connecting');
        } else {
            // Mostrar estado basado en si hay conexiones pendientes
            if (spinnerElements > 0) {
                this.updateStatus(`${serverCount} participantes (${videoElements} conectados, ${spinnerElements} conectando...)`, 'connecting');
            } else {
                this.updateStatus(`${serverCount} participantes en la sala`, 'connected');
            }
        }

        // Debug: solo advertir si hay diferencias significativas
        if (totalElements !== serverCount && spinnerElements === 0) {
            this.log(`⚠️ Discrepancia sin conexiones pendientes: Servidor dice ${serverCount}, pero DOM muestra ${totalElements} elementos`);
        }
    }

    sendSignalingMessage(message) {
        if (this.ws && this.ws.readyState === WebSocket.OPEN) {
            // Preparar mensaje para múltiples targets
            const msg = { ...message };

            // Manejar diferentes tipos de targets
            if (Array.isArray(msg.target)) {
                // Array de targets específicos
                msg.targets = msg.target;
                delete msg.target;
                this.log('Enviando mensaje:', msg.type, 'a múltiples targets:', msg.targets);
            } else if (msg.target === 'all') {
                // Broadcast a todos
                delete msg.target;
                this.log('Enviando mensaje broadcast:', msg.type, 'a toda la sala');
            } else if (msg.target) {
                // Target único (comportamiento actual)
                this.log('Enviando mensaje:', msg.type, 'a:', msg.target);
            } else {
                // Sin target = broadcast
                this.log('Enviando mensaje broadcast:', msg.type);
            }

            this.ws.send(JSON.stringify(msg));
        } else {
            this.log('WebSocket no está conectado');
        }
    }

    updateStatus(message, type) {
        const statusEl = document.getElementById('status');
        statusEl.textContent = message;
        statusEl.className = `status ${type}`;
        this.log('Status actualizado:', message);
    }

    updateVideoStatus(peerId, micOn, camOn) {
        const statusEl = document.getElementById(`status-${peerId}`);
        if (statusEl) {
            statusEl.innerHTML = '';

            if (!micOn) {
                const micIcon = document.createElement('span');
                micIcon.className = 'status-icon muted';
                micIcon.textContent = '🔇';
                statusEl.appendChild(micIcon);
            }

            if (!camOn) {
                const camIcon = document.createElement('span');
                camIcon.className = 'status-icon cam-off';
                camIcon.textContent = '📷';
                statusEl.appendChild(camIcon);
            }
        }
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

        // Actualizar status visual local
        this.updateVideoStatus('local', this.isMicOn, this.isCamOn);

        // Notificar a todos los peers sobre el cambio de estado (BROADCAST)
        this.sendSignalingMessage({
            type: 'user_media_changed',
            data: {
                peerId: this.myPeerId,
                micOn: this.isMicOn,
                camOn: this.isCamOn
            },
            target: 'all'  // ← Broadcast a todos
        });

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

        // Actualizar status visual local
        this.updateVideoStatus('local', this.isMicOn, this.isCamOn);

        // Notificar a todos los peers sobre el cambio de estado (BROADCAST)
        this.sendSignalingMessage({
            type: 'user_media_changed',
            data: {
                peerId: this.myPeerId,
                micOn: this.isMicOn,
                camOn: this.isCamOn
            },
            target: 'all'  // ← Broadcast a todos
        });

        this.log('Cámara:', this.isCamOn ? 'activada' : 'desactivada');
    }

    hangup() {
        this.log('Colgando llamada...');

        // Cerrar todas las conexiones peer
        this.peers.forEach((peer, peerId) => {
            if (peer.pc) {
                peer.pc.close();
            }
        });
        this.peers.clear();

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