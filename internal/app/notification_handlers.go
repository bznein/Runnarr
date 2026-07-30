package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

func (s *Server) handleListNotifications(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	page, err := s.store.ListNotifications(r.Context(), limit, r.URL.Query().Get("cursor"), r.URL.Query().Get("unread") == "true")
	if errors.Is(err, ErrInvalidNotification) {
		writeError(w, http.StatusBadRequest, "invalid notification cursor")
		return
	}
	if err != nil {
		s.logger.Error("list notifications", "error", err)
		writeError(w, http.StatusInternalServerError, "could not list notifications")
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) handleGetNotification(w http.ResponseWriter, r *http.Request) {
	item, err := s.store.GetNotification(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "notification not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load notification")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleUpdateNotification(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Read *bool `json:"read"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Read == nil {
		writeError(w, http.StatusBadRequest, "read is required")
		return
	}
	if err := s.store.SetNotificationRead(r.Context(), chi.URLParam(r, "id"), *body.Read); errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "notification not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update notification")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"updated": true})
}

func (s *Server) handleDeleteNotification(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteNotification(r.Context(), chi.URLParam(r, "id")); errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "notification not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete notification")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

func (s *Server) handleMarkAllNotificationsRead(w http.ResponseWriter, r *http.Request) {
	if err := s.store.MarkAllNotificationsRead(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "could not mark notifications read")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"updated": true})
}

func (s *Server) handleClearNotifications(w http.ResponseWriter, r *http.Request) {
	scope := strings.TrimSpace(r.URL.Query().Get("scope"))
	if scope != "all" && scope != "read" {
		writeError(w, http.StatusBadRequest, "scope must be read or all")
		return
	}
	if err := s.store.ClearNotifications(r.Context(), scope == "read"); err != nil {
		writeError(w, http.StatusInternalServerError, "could not clear notifications")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

func (s *Server) handleGetNotificationSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.store.GetNotificationSettings(r.Context(), s.webPushPublicKey())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load notification settings")
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) handleUpdateNotificationSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Categories map[string]string `json:"categories"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Categories == nil {
		writeError(w, http.StatusBadRequest, "categories are required")
		return
	}
	if err := s.store.UpdateNotificationSettings(r.Context(), body.Categories); errors.Is(err, ErrInvalidNotification) {
		writeError(w, http.StatusBadRequest, "invalid notification category or mode")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save notification settings")
		return
	}
	settings, err := s.store.GetNotificationSettings(r.Context(), s.webPushPublicKey())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load notification settings")
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func supportModeRequest(r *http.Request) bool {
	principal, err := principalFromContext(r.Context())
	return err == nil && principal.SupportTarget
}

func (s *Server) handleListWebPushSubscriptions(w http.ResponseWriter, r *http.Request) {
	if supportModeRequest(r) {
		writeError(w, http.StatusForbidden, "push devices are unavailable in support mode")
		return
	}
	items, err := s.store.ListWebPushSubscriptions(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list push devices")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"subscriptions": items})
}

func (s *Server) handleCreateWebPushSubscription(w http.ResponseWriter, r *http.Request) {
	if supportModeRequest(r) {
		writeError(w, http.StatusForbidden, "push devices are unavailable in support mode")
		return
	}
	var body struct {
		Endpoint       string            `json:"endpoint"`
		ExpirationTime *int64            `json:"expirationTime"`
		Keys           map[string]string `json:"keys"`
		DeviceName     string            `json:"deviceName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	item, err := s.saveWebPushSubscription(r.Context(), body.Endpoint, body.ExpirationTime, body.Keys, body.DeviceName, r.UserAgent())
	if errors.Is(err, ErrInvalidNotification) {
		writeError(w, http.StatusBadRequest, "invalid push subscription")
		return
	}
	if err != nil {
		s.logger.Error("save push subscription", "error", err)
		writeError(w, http.StatusInternalServerError, "could not enable push notifications")
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) handleUpdateWebPushSubscription(w http.ResponseWriter, r *http.Request) {
	if supportModeRequest(r) {
		writeError(w, http.StatusForbidden, "push devices are unavailable in support mode")
		return
	}
	var body struct {
		DeviceName string `json:"deviceName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := s.store.RenameWebPushSubscription(r.Context(), chi.URLParam(r, "id"), body.DeviceName); errors.Is(err, ErrInvalidNotification) {
		writeError(w, http.StatusBadRequest, "invalid device name")
		return
	} else if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "push device not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not rename push device")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"updated": true})
}

func (s *Server) handleDeleteWebPushSubscription(w http.ResponseWriter, r *http.Request) {
	if supportModeRequest(r) {
		writeError(w, http.StatusForbidden, "push devices are unavailable in support mode")
		return
	}
	if err := s.store.DeleteWebPushSubscription(r.Context(), chi.URLParam(r, "id")); errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "push device not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not remove push device")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

func (s *Server) handleDeleteCurrentWebPushSubscription(w http.ResponseWriter, r *http.Request) {
	if supportModeRequest(r) {
		writeError(w, http.StatusForbidden, "push devices are unavailable in support mode")
		return
	}
	var body struct {
		Endpoint string `json:"endpoint"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Endpoint) == "" {
		writeError(w, http.StatusBadRequest, "endpoint is required")
		return
	}
	if err := s.store.DeleteWebPushSubscriptionByEndpoint(r.Context(), body.Endpoint); err != nil {
		writeError(w, http.StatusInternalServerError, "could not remove push device")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

func (s *Server) handleTestWebPushSubscription(w http.ResponseWriter, r *http.Request) {
	if supportModeRequest(r) {
		writeError(w, http.StatusForbidden, "push devices are unavailable in support mode")
		return
	}
	if err := s.testWebPushSubscription(r.Context(), chi.URLParam(r, "id")); errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "push device not found")
		return
	} else if err != nil {
		s.logger.Error("test push notification", "error", err)
		writeError(w, http.StatusBadGateway, "test notification could not be delivered")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"delivered": true})
}
