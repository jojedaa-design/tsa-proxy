package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/bigdavi/tsa-proxy/internal/config"
)

type Server struct {
	httpServer *http.Server
	cfg        *config.Config
}

func New(cfg *config.Config, handler http.Handler) *Server {
	return &Server{
		cfg: cfg,
		httpServer: &http.Server{
			Addr:         fmt.Sprintf(":%s", cfg.Server.Port),
			Handler:      handler,
			ReadTimeout:  cfg.Server.ReadTimeout,
			WriteTimeout: cfg.Server.WriteTimeout,
			IdleTimeout:  cfg.Server.IdleTimeout,
		},
	}
}

// Start arranca el servidor HTTP. Bloqueante hasta que se cierre.
func (s *Server) Start() error {
	log.Info().
		Str("addr", s.httpServer.Addr).
		Str("env", s.cfg.Environment).
		Str("version", s.cfg.Version).
		Msg("starting HTTP server")

	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}
	return nil
}

// Shutdown realiza un graceful shutdown esperando que los requests en vuelo terminen.
func (s *Server) Shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), s.cfg.Server.ShutdownTimeout)
	defer cancel()

	log.Info().Msg("shutting down HTTP server gracefully")

	if err := s.httpServer.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("server shutdown error")
	} else {
		log.Info().Msg("server shutdown complete")
	}

	// Esperar que todos los handlers activos liberen sus recursos
	time.Sleep(100 * time.Millisecond)
}
