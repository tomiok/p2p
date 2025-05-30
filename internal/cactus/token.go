package cactus

import (
	"fmt"
	"github.com/livekit/protocol/auth"
	"time"

	"videocall/internal/config"
)

type TokenService struct {
	config *config.Config
}

func NewTokenService(cfg *config.Config) *TokenService {
	return &TokenService{
		config: cfg,
	}
}

func (s *TokenService) GenerateToken(roomID, participantName string) (string, error) {
	if roomID == "" {
		return "", fmt.Errorf("room ID is required")
	}
	if participantName == "" {
		return "", fmt.Errorf("participant name is required")
	}

	at := auth.NewAccessToken(s.config.LiveKitKey, s.config.LiveKitSecret)

	grant := &auth.VideoGrant{
		RoomJoin: true,
		Room:     roomID,
	}

	at.AddGrant(grant).
		SetIdentity(participantName).
		SetValidFor(time.Hour * 6) // Token valid for 6 hours

	token, err := at.ToJWT()
	if err != nil {
		return "", fmt.Errorf("failed to generate JWT token: %w", err)
	}

	return token, nil
}
