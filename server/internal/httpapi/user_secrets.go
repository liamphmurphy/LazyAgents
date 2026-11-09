package httpapi

import (
	"github.com/liamphmurphy/vesper/server/internal/secrets"
	"github.com/liamphmurphy/vesper/server/internal/usersecrets"
)

func (s *Server) userSecrets() *usersecrets.Service {
	var box *secrets.Box
	if s.Secrets != nil {
		box = s.Secrets.Box
	}
	return &usersecrets.Service{Q: s.Q, Box: box}
}
