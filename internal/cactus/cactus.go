package cactus

import (
	"context"
	"fmt"
	"github.com/livekit/protocol/livekit"
	lksdk "github.com/livekit/server-sdk-go"
	"math/rand"
	"sync"
	"time"
	"videocall/internal/config"
)

type Room struct {
	ID                 string    `json:"id"`
	Name               string    `json:"name,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	MaxParticipants    int       `json:"max_participants"`
	ActiveParticipants int       `json:"active_participants"`
	IsActive           bool      `json:"is_active"`
	CreatedBy          string    `json:"created_by,omitempty"`
}

type Participant struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	RoomID   string    `json:"room_id"`
	JoinedAt time.Time `json:"joined_at"`
	IsActive bool      `json:"is_active"`
}

type JoinRoomRequest struct {
	Name string `json:"name" validate:"required,min=1,max=50"`
}

type CreateRoomRequest struct {
	Name            string `json:"name,omitempty"`
	MaxParticipants int    `json:"max_participants,omitempty"`
}

type RoomResponse struct {
	Room    *Room  `json:"room"`
	JoinURL string `json:"join_url"`
}

type JoinRoomResponse struct {
	Token      string `json:"token"`
	LiveKitURL string `json:"livekit_url"`
	Room       *Room  `json:"room"`
}

type RoomService struct {
	config *config.Config
	client *lksdk.RoomServiceClient
	rooms  map[string]*Room
	mu     sync.RWMutex
}

func NewRoomService(cfg *config.Config) *RoomService {
	client := lksdk.NewRoomServiceClient(cfg.LiveKitURL, cfg.LiveKitKey, cfg.LiveKitSecret)

	return &RoomService{
		config: cfg,
		client: client,
		rooms:  make(map[string]*Room),
	}
}

func (s *RoomService) CreateRoom(ctx context.Context, req *CreateRoomRequest) (*Room, error) {
	roomID := s.generateRoomID()

	maxParticipants := req.MaxParticipants
	if maxParticipants == 0 {
		maxParticipants = 20
	}

	// Create room in LiveKit
	livekitReq := &livekit.CreateRoomRequest{
		Name:            roomID,
		MaxParticipants: uint32(maxParticipants),
		EmptyTimeout:    300, // 5 minutes
	}

	_, err := s.client.CreateRoom(ctx, livekitReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create LiveKit room: %w", err)
	}

	// Create room model
	room := &Room{
		ID:                 roomID,
		Name:               req.Name,
		CreatedAt:          time.Now(),
		MaxParticipants:    maxParticipants,
		ActiveParticipants: 0,
		IsActive:           true,
	}

	// Store room
	s.mu.Lock()
	s.rooms[roomID] = room
	s.mu.Unlock()

	return room, nil
}

func (s *RoomService) GetRoom(ctx context.Context, roomID string) (*Room, error) {
	s.mu.RLock()
	room, exists := s.rooms[roomID]
	s.mu.RUnlock()

	if !exists {
		// Try to fetch from LiveKit
		livekitRooms, err := s.client.ListRooms(ctx, &livekit.ListRoomsRequest{})
		if err != nil {
			return nil, fmt.Errorf("room not found")
		}

		// Check if room exists in LiveKit
		for _, lr := range livekitRooms.Rooms {
			if lr.Name == roomID {
				// Create room model from LiveKit data
				room = &Room{
					ID:                 roomID,
					CreatedAt:          time.Unix(lr.CreationTime, 0),
					MaxParticipants:    int(lr.MaxParticipants),
					ActiveParticipants: int(lr.NumParticipants),
					IsActive:           true,
				}

				// Store room
				s.mu.Lock()
				s.rooms[roomID] = room
				s.mu.Unlock()

				return room, nil
			}
		}

		return nil, fmt.Errorf("room not found")
	}

	// Update participant count from LiveKit
	if room.IsActive {
		if err := s.updateRoomParticipants(ctx, room); err != nil {
			// Log error but don't fail
			fmt.Printf("Failed to update room participants: %v\n", err)
		}
	}

	return room, nil
}

func (s *RoomService) DeleteRoom(ctx context.Context, roomID string) error {
	// Delete from LiveKit
	_, err := s.client.DeleteRoom(ctx, &livekit.DeleteRoomRequest{
		Room: roomID,
	})

	if err != nil {
		return fmt.Errorf("failed to delete LiveKit room: %w", err)
	}

	// Remove from local storage
	s.mu.Lock()
	delete(s.rooms, roomID)
	s.mu.Unlock()

	return nil
}

func (s *RoomService) updateRoomParticipants(ctx context.Context, room *Room) error {
	participants, err := s.client.ListParticipants(ctx, &livekit.ListParticipantsRequest{
		Room: room.ID,
	})
	if err != nil {
		return err
	}

	s.mu.Lock()
	room.ActiveParticipants = len(participants.Participants)
	s.mu.Unlock()

	return nil
}

func (s *RoomService) generateRoomID() string {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	const length = 6

	rand.New(rand.NewSource(time.Now().UnixNano()))
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}
