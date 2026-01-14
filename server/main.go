package server

import (
	"errors"
	"net/http"

	"github.com/selfdrivingcarp/tcwh"
)

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	sub := getSub(r.Context())

	us, err := s.ctrl.GetState(r.Context(), sub)
	if err != nil && !errors.Is(err, tcwh.ErrNotFound) {
		s.log.Error("getting oauth", "user", sub, "error", err.Error())
		quickResponse(w, http.StatusInternalServerError)
		return
	}

	s.sendJSON(w, us)
}
