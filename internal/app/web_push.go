package app

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	webpush "github.com/marknefedov/go-webpush/v2"
)

type webPushService struct {
	store   *Store
	cipher  *TokenCipher
	client  *webpush.Client
	keys    *webpush.VAPIDKeys
	subject string
	logger  *slog.Logger
}

type storedPushSubscription struct {
	Endpoint       string            `json:"endpoint"`
	ExpirationTime *int64            `json:"expirationTime,omitempty"`
	Keys           map[string]string `json:"keys"`
}

func newWebPushService(ctx context.Context, cfg Config, store *Store, logger *slog.Logger) (*webPushService, error) {
	cipher, err := NewTokenCipher(cfg.EncryptionKey())
	if err != nil {
		return nil, fmt.Errorf("create web push cipher: %w", err)
	}
	privateCiphertext, publicKey, err := store.WebPushKeys(ctx)
	if err != nil {
		return nil, fmt.Errorf("load web push keys: %w", err)
	}
	var keys *webpush.VAPIDKeys
	if len(privateCiphertext) == 0 || publicKey == "" {
		keys, err = webpush.GenerateVAPIDKeys()
		if err != nil {
			return nil, fmt.Errorf("generate web push keys: %w", err)
		}
		encoded, err := json.Marshal(keys)
		if err != nil {
			return nil, fmt.Errorf("encode web push keys: %w", err)
		}
		privateCiphertext, err = cipher.EncryptString(string(encoded))
		if err != nil {
			return nil, fmt.Errorf("encrypt web push keys: %w", err)
		}
		publicKey = keys.PublicKeyString()
		if err := store.SaveWebPushKeys(ctx, privateCiphertext, publicKey); err != nil {
			return nil, fmt.Errorf("save web push keys: %w", err)
		}
	} else {
		encoded, err := cipher.DecryptString(privateCiphertext)
		if err != nil {
			return nil, fmt.Errorf("decrypt web push keys: %w", err)
		}
		keys = &webpush.VAPIDKeys{}
		if err := json.Unmarshal([]byte(encoded), keys); err != nil {
			return nil, fmt.Errorf("decode web push keys: %w", err)
		}
		if keys.PublicKeyString() != publicKey {
			return nil, errors.New("stored web push key pair does not match")
		}
	}

	subject := "https://github.com/bznein/Runnarr"
	if parsed, err := url.Parse(cfg.BaseURL); err == nil && parsed.Scheme == "https" && parsed.Host != "" {
		subject = cfg.BaseURL
	}
	httpClient := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			DialContext:           restrictedPushDialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          20,
			IdleConnTimeout:       30 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
		},
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return errors.New("web push redirects are not allowed")
		},
	}
	return &webPushService{
		store: store, cipher: cipher, keys: keys, subject: subject, logger: logger,
		client: webpush.NewClient(webpush.Config{HTTPClient: httpClient, MaxConcurrentSends: 4}),
	}, nil
}

func validatePushEndpoint(endpoint string) error {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return ErrInvalidNotification
	}
	host := parsed.Hostname()
	if host == "" || strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return ErrInvalidNotification
	}
	if parsed.Port() != "" {
		port, err := strconv.Atoi(parsed.Port())
		if err != nil || port != 443 {
			return ErrInvalidNotification
		}
	}
	if ip := net.ParseIP(host); ip != nil && !publicPushIP(ip) {
		return ErrInvalidNotification
	}
	return nil
}

func publicPushIP(ip net.IP) bool {
	address, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	address = address.Unmap()
	for _, prefix := range nonPublicPushPrefixes {
		if prefix.Contains(address) {
			return false
		}
	}
	return address.IsGlobalUnicast()
}

var nonPublicPushPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/128"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("ff00::/8"),
}

func restrictedPushDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(addresses) == 0 {
		return nil, errors.New("web push endpoint did not resolve")
	}
	for _, address := range addresses {
		if !publicPushIP(address.IP) {
			return nil, errors.New("web push endpoint resolved to a non-public address")
		}
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	return dialer.DialContext(ctx, network, net.JoinHostPort(addresses[0].IP.String(), port))
}

func (s *Server) webPushPublicKey() string {
	if s.webPush == nil || s.webPush.keys == nil {
		return ""
	}
	return s.webPush.keys.PublicKeyString()
}

func (s *Server) saveWebPushSubscription(ctx context.Context, endpoint string, expirationTime *int64, keys map[string]string, deviceName, userAgent string) (WebPushSubscription, error) {
	if s.webPush == nil || validatePushEndpoint(endpoint) != nil || strings.TrimSpace(keys["auth"]) == "" || strings.TrimSpace(keys["p256dh"]) == "" {
		return WebPushSubscription{}, ErrInvalidNotification
	}
	stored := storedPushSubscription{Endpoint: strings.TrimSpace(endpoint), ExpirationTime: expirationTime, Keys: keys}
	encoded, err := json.Marshal(stored)
	if err != nil {
		return WebPushSubscription{}, err
	}
	var parsed webpush.Subscription
	if err := json.Unmarshal(encoded, &parsed); err != nil {
		return WebPushSubscription{}, ErrInvalidNotification
	}
	ciphertext, err := s.webPush.cipher.EncryptString(string(encoded))
	if err != nil {
		return WebPushSubscription{}, err
	}
	return s.store.SaveWebPushSubscription(ctx, stored.Endpoint, ciphertext, deviceName, userAgent)
}

func (s *webPushService) decodeSubscription(ciphertext []byte) (*webpush.Subscription, error) {
	encoded, err := s.cipher.DecryptString(ciphertext)
	if err != nil {
		return nil, err
	}
	var subscription webpush.Subscription
	if err := json.Unmarshal([]byte(encoded), &subscription); err != nil {
		return nil, err
	}
	if err := validatePushEndpoint(subscription.Endpoint); err != nil {
		return nil, err
	}
	return &subscription, nil
}

func (s *webPushService) send(ctx context.Context, subscription *webpush.Subscription, payload []byte) (int, error) {
	var metadata struct {
		Severity string `json:"severity"`
		Tag      string `json:"tag"`
	}
	_ = json.Unmarshal(payload, &metadata)
	urgency := webpush.UrgencyNormal
	if metadata.Severity == "warning" || metadata.Severity == "error" {
		urgency = webpush.UrgencyHigh
	}
	topicHash := sha256Text(metadata.Tag)
	result, err := s.client.Send(ctx, payload, subscription, webpush.SendOptions{
		Subject: s.subject, TTL: 24 * 60 * 60, Urgency: urgency, Topic: topicHash[:32], VAPIDKeys: s.keys,
	})
	if err != nil {
		var serviceError *webpush.PushServiceError
		if errors.As(err, &serviceError) {
			return serviceError.StatusCode, err
		}
		return 0, err
	}
	if result.Response != nil {
		defer result.Response.Body.Close()
		if result.StatusCode < 200 || result.StatusCode >= 300 {
			body, _ := io.ReadAll(io.LimitReader(result.Response.Body, 2048))
			return result.StatusCode, fmt.Errorf("push service returned %d: %s", result.StatusCode, strings.TrimSpace(string(body)))
		}
	}
	return result.StatusCode, nil
}

func sha256Text(value string) string {
	sum := sha256.Sum256([]byte(value))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func (s *Server) testWebPushSubscription(ctx context.Context, id string) error {
	if s.webPush == nil {
		return errors.New("web push is unavailable")
	}
	ciphertext, err := s.store.GetWebPushSubscriptionCiphertext(ctx, id)
	if err != nil {
		return err
	}
	subscription, err := s.webPush.decodeSubscription(ciphertext)
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]any{
		"title": "Runnarr notifications are working", "body": "This device can receive updates while Runnarr is closed.",
		"severity": "success", "actionPath": "/settings?section=notifications", "tag": "runnarr-push-test", "unreadCount": 0,
	})
	status, err := s.webPush.send(ctx, subscription, payload)
	if err != nil {
		var serviceError *webpush.PushServiceError
		if status == http.StatusNotFound || status == http.StatusGone || (errors.As(err, &serviceError) && serviceError.SubscriptionExpired) {
			_ = s.store.ExpireWebPushSubscription(ctx, id)
		} else {
			_ = s.store.RecordWebPushSubscriptionError(ctx, id, err.Error())
		}
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("push service returned %d", status)
	}
	return s.store.RecordWebPushSubscriptionSuccess(ctx, id)
}

func (s *Server) runWebPushDispatcher(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	s.dispatchWebPush(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.dispatchWebPush(ctx)
		}
	}
}

func (s *Server) dispatchWebPush(ctx context.Context) {
	if s.webPush == nil {
		return
	}
	items, err := s.store.ClaimWebPushOutbox(ctx, 20)
	if err != nil {
		s.logger.Error("claim web push outbox", "error", err)
		return
	}
	for _, item := range items {
		s.dispatchWebPushItem(ctx, item)
	}
}

func (s *Server) dispatchWebPushItem(ctx context.Context, item webPushOutboxItem) {
	subscription, err := s.webPush.decodeSubscription(item.SubscriptionCiphertext)
	if err != nil {
		_ = s.store.ExpireWebPushSubscription(ctx, item.SubscriptionID)
		return
	}
	status, err := s.webPush.send(ctx, subscription, item.Payload)
	if err == nil && status >= 200 && status < 300 {
		if err := s.store.CompleteWebPushOutbox(ctx, item); err != nil {
			s.logger.Error("complete web push delivery", "error", err)
		}
		return
	}
	if status == http.StatusNotFound || status == http.StatusGone {
		_ = s.store.ExpireWebPushSubscription(ctx, item.SubscriptionID)
		return
	}
	retryAfter := time.Duration(0)
	if status == 0 || status == http.StatusTooManyRequests || status >= 500 {
		delays := []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute}
		if item.Attempts < len(delays) {
			retryAfter = delays[item.Attempts]
		}
	}
	message := "web push delivery failed"
	if err != nil {
		message = err.Error()
	}
	if err := s.store.FailWebPushOutbox(ctx, item, message, retryAfter); err != nil {
		s.logger.Error("record web push delivery failure", "error", err)
	}
}

func (s *Server) runNotificationMaintenance(ctx context.Context) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.store.PruneNotifications(ctx, time.Now().UTC().AddDate(0, 0, -90)); err != nil {
				s.logger.Error("prune notifications", "error", err)
			}
		}
	}
}
