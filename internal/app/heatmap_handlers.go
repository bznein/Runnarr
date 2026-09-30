package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image/png"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

func parseHeatmapFilters(q url.Values) (heatmapFilters, error) {
	f := heatmapFilters{From: q.Get("from"), To: q.Get("to"), Timezone: q.Get("timezone"), Sports: q["sport"]}
	if f.Timezone == "" {
		f.Timezone = "UTC"
	}
	if _, err := time.LoadLocation(f.Timezone); err != nil {
		return f, fmt.Errorf("invalid timezone")
	}
	for _, date := range []string{f.From, f.To} {
		if date != "" {
			if _, err := time.Parse("2006-01-02", date); err != nil {
				return f, fmt.Errorf("dates must use YYYY-MM-DD")
			}
		}
	}
	if f.From != "" && f.To != "" && f.From > f.To {
		return f, fmt.Errorf("start date must not follow end date")
	}
	if len(f.Sports) == 0 {
		f.Sports = []string{"running"}
	}
	if len(f.Sports) > 50 {
		return f, fmt.Errorf("too many sports")
	}
	for _, sport := range f.Sports {
		if sport == "" || len(sport) > 100 || ((sport == "all" || sport == "running") && len(f.Sports) > 1) {
			return f, fmt.Errorf("invalid sport filter")
		}
	}
	sort.Strings(f.Sports)
	return f, nil
}

func (s *Server) handleHeatmap(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	filters, err := parseHeatmapFilters(r.URL.Query())
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	metadata, err := s.store.heatmapMetadata(r.Context(), filters)
	if err != nil {
		if r.Context().Err() != nil {
			return
		}
		s.logger.Error("read heatmap", "error", err)
		writeError(w, 500, "Could not load heatmap")
		return
	}
	writeJSON(w, 200, metadata)
}

func (s *Server) handleHeatmapTile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	q := r.URL.Query()
	filters, err := parseHeatmapFilters(q)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	z, ez := strconv.Atoi(chi.URLParam(r, "z"))
	x, ex := strconv.Atoi(chi.URLParam(r, "x"))
	y, ey := strconv.Atoi(strings.TrimSuffix(chi.URLParam(r, "y"), ".png"))
	scale := 1
	if q.Get("scale") != "" {
		scale, err = strconv.Atoi(q.Get("scale"))
	}
	if ez != nil || ex != nil || ey != nil || err != nil || z < 0 || z > 18 || x < 0 || y < 0 || x >= 1<<z || y >= 1<<z || (scale != 1 && scale != 2) {
		writeError(w, 400, "Invalid heatmap tile")
		return
	}
	appearance := q.Get("appearance")
	if appearance != "light" && appearance != "dark" {
		writeError(w, 400, "Invalid appearance")
		return
	}
	revision, err := strconv.ParseInt(q.Get("revision"), 10, 64)
	if err != nil || revision < 0 {
		writeError(w, 400, "Invalid revision")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	select {
	case s.heatTiles.slots <- struct{}{}:
		defer func() { <-s.heatTiles.slots }()
	case <-ctx.Done():
		if r.Context().Err() == nil {
			writeError(w, http.StatusGatewayTimeout, "Heatmap rendering is busy; please retry")
		}
		return
	}
	tx, err := s.store.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		writeError(w, 500, "Could not load heatmap tile")
		return
	}
	defer tx.Rollback(ctx)
	user := scopedUserID(ctx)
	current, err := heatmapRevision(ctx, tx, user)
	if err != nil {
		writeError(w, 500, "Could not load heatmap revision")
		return
	}
	if current != revision {
		writeError(w, 409, "Heatmap changed; refresh to load the latest routes")
		return
	}
	keyBytes, _ := json.Marshal([]any{user, filters, revision, z, x, y, scale, appearance, heatmapIndexVersion})
	key := string(keyBytes)
	data := s.heatTiles.get(key)
	if data == nil {
		raster, renderErr := renderHeatmapRows(ctx, tx, filters, user, z, x, y, scale)
		if renderErr != nil {
			if r.Context().Err() != nil {
				return
			}
			s.logger.Error("render heatmap tile", "error", renderErr)
			writeError(w, 500, "Could not render heatmap tile")
			return
		}
		var buffer bytes.Buffer
		encoder := png.Encoder{CompressionLevel: png.BestSpeed}
		if err = encoder.Encode(&buffer, raster.image(appearance == "dark")); err != nil {
			writeError(w, 500, "Could not encode heatmap tile")
			return
		}
		data = buffer.Bytes()
		s.heatTiles.put(key, data)
	}
	if err = tx.Commit(ctx); err != nil {
		writeError(w, 500, "Could not finish heatmap tile")
		return
	}
	if ctx.Err() != nil {
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("X-Heatmap-Revision", strconv.FormatInt(revision, 10))
	_, _ = w.Write(data)
}
