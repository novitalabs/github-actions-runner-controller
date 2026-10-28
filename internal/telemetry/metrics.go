package telemetry

import (
	"fmt"
	"net/http"
)

type Source interface {
	Metrics() (uint64, uint64, uint64)
}

func Handler(s Source) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		c, d, e := s.Metrics()
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		fmt.Fprintf(w, "novita_runners_created_total %d\nnovita_runners_deleted_total %d\nnovita_controller_errors_total %d\n", c, d, e)
	})
}
