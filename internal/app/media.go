package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rwcarlsen/goexif/exif"
	xdraw "golang.org/x/image/draw"
)

const (
	maxMediaUploadBytes = 25 << 20
	mediaThumbnailMaxPx = 480
)

var (
	ErrEmptyMediaFile       = errors.New("media file is empty")
	ErrMediaFileTooLarge    = errors.New("media file must be 25 MiB or smaller")
	ErrUnsupportedMediaType = errors.New("media must be a JPEG or PNG image")
	ErrUnsafeMediaPath      = errors.New("invalid media storage path")
)

type MediaService struct {
	store    *Store
	mediaDir string
}

type imageMetadata struct {
	Width       int
	Height      int
	CaptureTime *time.Time
	Latitude    *float64
	Longitude   *float64
	Orientation int
}

func NewMediaService(cfg Config, store *Store) *MediaService {
	return &MediaService{
		store:    store,
		mediaDir: cfg.MediaDir,
	}
}

func (s *MediaService) UploadActivityMedia(ctx context.Context, activityID, filename string, reader io.Reader) (ActivityMedia, error) {
	exists, err := s.store.ActivityExists(ctx, activityID)
	if err != nil {
		return ActivityMedia{}, err
	}
	if !exists {
		return ActivityMedia{}, pgx.ErrNoRows
	}

	data, err := readMediaUpload(reader)
	if err != nil {
		return ActivityMedia{}, err
	}
	contentType, extension, err := supportedMediaType(data)
	if err != nil {
		return ActivityMedia{}, err
	}

	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	if existing, err := s.store.GetActivityMediaByHash(ctx, activityID, hash); err == nil {
		return existing, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return ActivityMedia{}, err
	}

	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return ActivityMedia{}, ErrUnsupportedMediaType
	}
	metadata := extractImageMetadata(data, contentType)
	oriented := orientImage(decoded, metadata.Orientation)
	metadata.Width = oriented.Bounds().Dx()
	metadata.Height = oriented.Bounds().Dy()

	thumbnail, err := thumbnailJPEG(oriented, mediaThumbnailMaxPx)
	if err != nil {
		return ActivityMedia{}, err
	}

	originalPath := filepath.ToSlash(filepath.Join(activityID, "original", hash+extension))
	thumbnailPath := filepath.ToSlash(filepath.Join(activityID, "thumbnail", hash+".jpg"))
	if err := s.writeMediaFile(originalPath, data); err != nil {
		return ActivityMedia{}, err
	}
	if err := s.writeMediaFile(thumbnailPath, thumbnail); err != nil {
		_ = s.removeMediaFile(originalPath)
		return ActivityMedia{}, err
	}

	media, err := s.store.CreateActivityMedia(ctx, ActivityMedia{
		ActivityID:       activityID,
		OriginalFilename: cleanMediaFilename(filename, extension),
		ContentType:      contentType,
		SizeBytes:        int64(len(data)),
		SHA256:           hash,
		OriginalPath:     originalPath,
		ThumbnailPath:    thumbnailPath,
		Width:            metadata.Width,
		Height:           metadata.Height,
		CaptureTime:      metadata.CaptureTime,
		Latitude:         metadata.Latitude,
		Longitude:        metadata.Longitude,
	})
	if err != nil {
		_ = s.removeMediaFile(originalPath)
		_ = s.removeMediaFile(thumbnailPath)
		return ActivityMedia{}, err
	}
	return media, nil
}

func (s *MediaService) DeleteActivityMedia(ctx context.Context, activityID, mediaID string) (ActivityMedia, error) {
	media, err := s.store.DeleteActivityMedia(ctx, activityID, mediaID)
	if err != nil {
		return ActivityMedia{}, err
	}
	s.removeMediaFiles(media)
	return media, nil
}

func (s *MediaService) RemoveActivityMediaFiles(media []ActivityMedia) {
	for _, item := range media {
		s.removeMediaFiles(item)
	}
}

func (s *MediaService) OriginalFilePath(media ActivityMedia) (string, error) {
	return s.mediaPath(media.OriginalPath)
}

func (s *MediaService) ThumbnailFilePath(media ActivityMedia) (string, error) {
	return s.mediaPath(media.ThumbnailPath)
}

func readMediaUpload(reader io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxMediaUploadBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, ErrEmptyMediaFile
	}
	if len(data) > maxMediaUploadBytes {
		return nil, ErrMediaFileTooLarge
	}
	return data, nil
}

func supportedMediaType(data []byte) (string, string, error) {
	contentType := http.DetectContentType(data)
	switch contentType {
	case "image/jpeg":
		return contentType, ".jpg", nil
	case "image/png":
		return contentType, ".png", nil
	default:
		return "", "", ErrUnsupportedMediaType
	}
}

func cleanMediaFilename(filename, extension string) string {
	filename = strings.ReplaceAll(filename, "\\", "/")
	filename = filepath.Base(strings.TrimSpace(filename))
	if filename == "" || filename == "." || filename == string(filepath.Separator) {
		return "media" + extension
	}
	return filename
}

func extractImageMetadata(data []byte, contentType string) imageMetadata {
	if contentType != "image/jpeg" {
		return imageMetadata{Orientation: 1}
	}
	x, err := exif.Decode(bytes.NewReader(data))
	if err != nil {
		return imageMetadata{Orientation: 1}
	}

	metadata := imageMetadata{Orientation: exifOrientation(x)}
	if captured, err := x.DateTime(); err == nil && !captured.IsZero() {
		metadata.CaptureTime = &captured
	}
	if lat, lon, err := x.LatLong(); err == nil && validLatLong(lat, lon) {
		metadata.Latitude = &lat
		metadata.Longitude = &lon
	}
	return metadata
}

func exifOrientation(x *exif.Exif) int {
	tag, err := x.Get(exif.Orientation)
	if err != nil {
		return 1
	}
	orientation, err := tag.Int(0)
	if err != nil || orientation < 1 || orientation > 8 {
		return 1
	}
	return orientation
}

func validLatLong(lat, lon float64) bool {
	return !math.IsNaN(lat) && !math.IsNaN(lon) && lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180
}

func thumbnailJPEG(src image.Image, maxDimension int) ([]byte, error) {
	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return nil, ErrUnsupportedMediaType
	}

	targetWidth, targetHeight := scaledDimensions(width, height, maxDimension)
	dst := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, bounds, xdraw.Over, nil)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 82}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func scaledDimensions(width, height, maxDimension int) (int, int) {
	if width <= maxDimension && height <= maxDimension {
		return width, height
	}
	scale := float64(maxDimension) / float64(max(width, height))
	return max(1, int(math.Round(float64(width)*scale))), max(1, int(math.Round(float64(height)*scale)))
}

func orientImage(src image.Image, orientation int) image.Image {
	if orientation <= 1 || orientation > 8 {
		return src
	}
	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return src
	}

	dstWidth, dstHeight := width, height
	if orientation >= 5 && orientation <= 8 {
		dstWidth, dstHeight = height, width
	}
	dst := image.NewRGBA(image.Rect(0, 0, dstWidth, dstHeight))
	for y := 0; y < dstHeight; y++ {
		for x := 0; x < dstWidth; x++ {
			srcX, srcY := orientedSourcePoint(x, y, width, height, orientation)
			dst.Set(x, y, src.At(bounds.Min.X+srcX, bounds.Min.Y+srcY))
		}
	}
	return dst
}

func orientedSourcePoint(x, y, width, height, orientation int) (int, int) {
	switch orientation {
	case 2:
		return width - 1 - x, y
	case 3:
		return width - 1 - x, height - 1 - y
	case 4:
		return x, height - 1 - y
	case 5:
		return y, x
	case 6:
		return y, height - 1 - x
	case 7:
		return width - 1 - y, height - 1 - x
	case 8:
		return width - 1 - y, x
	default:
		return x, y
	}
}

func (s *MediaService) writeMediaFile(relativePath string, data []byte) error {
	path, err := s.mediaPath(relativePath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func (s *MediaService) removeMediaFiles(media ActivityMedia) {
	_ = s.removeMediaFile(media.OriginalPath)
	_ = s.removeMediaFile(media.ThumbnailPath)
}

func (s *MediaService) removeMediaFile(relativePath string) error {
	path, err := s.mediaPath(relativePath)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

func (s *MediaService) mediaPath(relativePath string) (string, error) {
	if strings.TrimSpace(s.mediaDir) == "" {
		return "", fmt.Errorf("RUNNARR_MEDIA_DIR is required")
	}
	clean := filepath.Clean(filepath.FromSlash(relativePath))
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", ErrUnsafeMediaPath
	}
	return filepath.Join(s.mediaDir, clean), nil
}
