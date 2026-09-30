package app

import (
	"container/list"
	"context"
	"image"
	"image/color"
	"math"
	"sync"
)

const heatTileSize = 256
const heatTilePadding = 4
const heatCacheBytes = 64 << 20
const heatCacheEntries = 4096 // Bound key/list overhead even for tiny empty PNGs.

// A nearest-value Gaussian lookup avoids an exponential for every pixel of
// every route. Quantization changes a contribution by less than 0.00007,
// below displayed color precision, while preserving the same distance kernel.
var heatWeights = func() [65537]uint16 {
	var weights [65537]uint16
	for i := range weights {
		weights[i] = uint16(math.Exp(-float64(i)/(4096*2.25)) * 65535)
	}
	return weights
}()

type heatRaster struct {
	size, scale    int
	origin         heatPoint
	pixelsPerMetre float64
	sum            []float32
	mask           []uint16
	touched        []int
}

func newHeatRaster(z, x, y, scale int) *heatRaster {
	size := (heatTileSize + 2*heatTilePadding) * scale
	tileMetres := heatmapWorld / math.Exp2(float64(z))
	pixelMetres := tileMetres / heatTileSize
	return &heatRaster{size: size, scale: scale,
		origin:         heatPoint{-heatmapWorld/2 + float64(x)*tileMetres - float64(heatTilePadding)*pixelMetres, heatmapWorld/2 - float64(y)*tileMetres + float64(heatTilePadding)*pixelMetres},
		pixelsPerMetre: float64(scale) / pixelMetres,
		sum:            make([]float32, size*size), mask: make([]uint16, size*size)}
}

func (r *heatRaster) addActivity(ctx context.Context, geometry heatGeometry) error {
	for _, line := range geometry.Coordinates {
		for i := 1; i < len(line); i++ {
			if i%128 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			a, b := line[i-1], line[i]
			a = heatPoint{(a[0] - r.origin[0]) * r.pixelsPerMetre, (r.origin[1] - a[1]) * r.pixelsPerMetre}
			b = heatPoint{(b[0] - r.origin[0]) * r.pixelsPerMetre, (r.origin[1] - b[1]) * r.pixelsPerMetre}
			r.segment(a, b)
		}
	}
	// At most one contribution per activity per pixel, including retraced laps.
	for _, pixel := range r.touched {
		r.sum[pixel] += float32(r.mask[pixel]) / 65535
		r.mask[pixel] = 0
	}
	r.touched = r.touched[:0]
	return ctx.Err()
}

func (r *heatRaster) segment(a, b heatPoint) {
	dx, dy := b[0]-a[0], b[1]-a[1]
	lengthSquared := dx*dx + dy*dy
	if lengthSquared == 0 {
		return
	}
	inverseLength := 1 / lengthSquared
	inverseScaleSquared := 1 / float64(r.scale*r.scale)
	// Small bounding boxes avoid scanning an entire tile for diagonal lines.
	steps := max(1, int(math.Ceil(math.Sqrt(lengthSquared)/float64(8*r.scale))))
	radius := float64(heatTilePadding * r.scale)
	for step := 0; step < steps; step++ {
		start, end := float64(step)/float64(steps), float64(step+1)/float64(steps)
		x1, y1, x2, y2 := a[0]+dx*start, a[1]+dy*start, a[0]+dx*end, a[1]+dy*end
		minX, maxX := max(0, int(math.Floor(math.Min(x1, x2)-radius))), min(r.size-1, int(math.Ceil(math.Max(x1, x2)+radius)))
		minY, maxY := max(0, int(math.Floor(math.Min(y1, y2)-radius))), min(r.size-1, int(math.Ceil(math.Max(y1, y2)+radius)))
		for y := minY; y <= maxY; y++ {
			for x := minX; x <= maxX; x++ {
				px, py := float64(x)+0.5-a[0], float64(y)+0.5-a[1]
				t := math.Min(1.0, math.Max(0.0, (px*dx+py*dy)*inverseLength))
				sx, sy := px-t*dx, py-t*dy
				distanceSquared := (sx*sx + sy*sy) * inverseScaleSquared
				if distanceSquared >= heatTilePadding*heatTilePadding {
					continue
				}
				weight := heatWeights[int(distanceSquared*4096+0.5)]
				pixel := y*r.size + x
				if weight > r.mask[pixel] {
					if r.mask[pixel] == 0 {
						r.touched = append(r.touched, pixel)
					}
					r.mask[pixel] = weight
				}
			}
		}
	}
}

func (r *heatRaster) image(dark bool) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, heatTileSize*r.scale, heatTileSize*r.scale))
	pad := heatTilePadding * r.scale
	for y := 0; y < out.Bounds().Dy(); y++ {
		for x := 0; x < out.Bounds().Dx(); x++ {
			value := float64(r.sum[(y+pad)*r.size+x+pad])
			if value < 0.001 {
				continue
			}
			fraction := math.Min(1.0, math.Log1p(value)/math.Log(101))
			palette := [4][3]float64{{104, 63, 163}, {181, 54, 122}, {237, 105, 37}, {252, 223, 110}}
			if !dark {
				palette = [4][3]float64{{93, 53, 153}, {164, 44, 114}, {215, 67, 37}, {151, 32, 18}}
			}
			position := fraction * 3
			lo := min(2, int(position))
			mix := position - float64(lo)
			c := color.NRGBA{A: uint8(255 * (0.60 + 0.35*fraction) * (1 - math.Exp(-value*3)))}
			c.R = uint8(palette[lo][0]*(1-mix) + palette[lo+1][0]*mix)
			c.G = uint8(palette[lo][1]*(1-mix) + palette[lo+1][1]*mix)
			c.B = uint8(palette[lo][2]*(1-mix) + palette[lo+1][2]*mix)
			out.SetNRGBA(x, y, c)
		}
	}
	return out
}

type heatCacheItem struct {
	key string
	png []byte
}
type heatTileService struct {
	slots   chan struct{}
	mu      sync.Mutex
	entries map[string]*list.Element
	lru     *list.List
	bytes   int
}

func newHeatTileService() *heatTileService {
	return &heatTileService{slots: make(chan struct{}, 4), entries: make(map[string]*list.Element), lru: list.New()}
}
func (s *heatTileService) get(key string) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry := s.entries[key]; entry != nil {
		s.lru.MoveToFront(entry)
		return entry.Value.(heatCacheItem).png
	}
	return nil
}
func (s *heatTileService) put(key string, png []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.entries[key]; exists {
		return
	}
	s.entries[key] = s.lru.PushFront(heatCacheItem{key, png})
	s.bytes += len(png)
	for s.bytes > heatCacheBytes || len(s.entries) > heatCacheEntries {
		tail := s.lru.Back()
		item := tail.Value.(heatCacheItem)
		s.bytes -= len(item.png)
		delete(s.entries, item.key)
		s.lru.Remove(tail)
	}
}
