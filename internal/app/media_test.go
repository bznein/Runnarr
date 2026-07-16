package app

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

func TestSupportedMediaType(t *testing.T) {
	jpegData := encodedJPEG(t, solidImage(8, 8, color.RGBA{R: 200, A: 255}))
	contentType, extension, err := supportedMediaType(jpegData)
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "image/jpeg" || extension != ".jpg" {
		t.Fatalf("JPEG detection = %q %q", contentType, extension)
	}

	pngData := encodedPNG(t, solidImage(8, 8, color.RGBA{G: 200, A: 255}))
	contentType, extension, err = supportedMediaType(pngData)
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "image/png" || extension != ".png" {
		t.Fatalf("PNG detection = %q %q", contentType, extension)
	}

	if _, _, err := supportedMediaType([]byte("not an image")); !errors.Is(err, ErrUnsupportedMediaType) {
		t.Fatalf("unsupported error = %v", err)
	}
}

func TestReadMediaUploadValidation(t *testing.T) {
	if _, err := readMediaUpload(strings.NewReader("")); !errors.Is(err, ErrEmptyMediaFile) {
		t.Fatalf("empty error = %v", err)
	}

	tooLarge := bytes.NewReader(bytes.Repeat([]byte{1}, maxMediaUploadBytes+1))
	if _, err := readMediaUpload(tooLarge); !errors.Is(err, ErrMediaFileTooLarge) {
		t.Fatalf("large error = %v", err)
	}
}

func TestThumbnailJPEGScalesToMaximumDimension(t *testing.T) {
	thumbnail, err := thumbnailJPEG(solidImage(1200, 600, color.RGBA{B: 200, A: 255}), 480)
	if err != nil {
		t.Fatal(err)
	}
	decoded, _, err := image.Decode(bytes.NewReader(thumbnail))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Bounds().Dx() != 480 || decoded.Bounds().Dy() != 240 {
		t.Fatalf("thumbnail size = %dx%d", decoded.Bounds().Dx(), decoded.Bounds().Dy())
	}
}

func TestMediaPathRejectsUnsafePaths(t *testing.T) {
	service := &MediaService{mediaDir: t.TempDir()}
	for _, path := range []string{"../outside.jpg", "/absolute.jpg", ".."} {
		if _, err := service.mediaPath(path); !errors.Is(err, ErrUnsafeMediaPath) {
			t.Fatalf("mediaPath(%q) error = %v", path, err)
		}
	}

	path, err := service.mediaPath("activity/original/photo.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(path, "activity") || !strings.Contains(path, "photo.jpg") {
		t.Fatalf("path = %q", path)
	}
}

func TestCleanMediaFilename(t *testing.T) {
	if got := cleanMediaFilename(`C:\Users\me\photo.JPG`, ".jpg"); got != "photo.JPG" {
		t.Fatalf("clean filename = %q", got)
	}
	if got := cleanMediaFilename("", ".png"); got != "media.png" {
		t.Fatalf("fallback filename = %q", got)
	}
}

func TestExtractImageMetadataWithoutEXIF(t *testing.T) {
	metadata := extractImageMetadata(encodedJPEG(t, solidImage(4, 4, color.White)), "image/jpeg")
	if metadata.Orientation != 1 {
		t.Fatalf("orientation = %d", metadata.Orientation)
	}
	if metadata.CaptureTime != nil || metadata.Latitude != nil || metadata.Longitude != nil {
		t.Fatalf("metadata should be empty without EXIF: %#v", metadata)
	}
}

func TestOrientImageRotatesClockwise(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 2, 1))
	src.Set(0, 0, color.RGBA{R: 255, A: 255})
	src.Set(1, 0, color.RGBA{B: 255, A: 255})

	rotated := orientImage(src, 6)
	if rotated.Bounds().Dx() != 1 || rotated.Bounds().Dy() != 2 {
		t.Fatalf("rotated size = %dx%d", rotated.Bounds().Dx(), rotated.Bounds().Dy())
	}
	if got := rotated.At(0, 0); !sameRGBA(got, color.RGBA{R: 255, A: 255}) {
		t.Fatalf("top pixel = %#v", got)
	}
	if got := rotated.At(0, 1); !sameRGBA(got, color.RGBA{B: 255, A: 255}) {
		t.Fatalf("bottom pixel = %#v", got)
	}
}

func encodedJPEG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func encodedPNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sameRGBA(left color.Color, right color.RGBA) bool {
	r, g, b, a := left.RGBA()
	return uint8(r>>8) == right.R && uint8(g>>8) == right.G && uint8(b>>8) == right.B && uint8(a>>8) == right.A
}

func solidImage(width, height int, c color.Color) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}
