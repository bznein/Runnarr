import L from "leaflet";
import { heatmapExportSize } from "./heatmap";

type LoadedTile = { coordinates: L.Coords; image: HTMLImageElement };
const tileKey = (coordinates: L.Coords) => `${coordinates.z}/${coordinates.x}/${coordinates.y}`;

export class HeatmapLayer extends L.GridLayer {
  readonly tiles = new Map<string, LoadedTile>();
  private controllers = new Map<HTMLElement, AbortController>();
  constructor(readonly query: string, readonly stale: () => void) {
    super({ tileSize: 256, minZoom: 0, maxZoom: 18, noWrap: true, keepBuffer: 1, updateWhenIdle: true, pane: "overlayPane" });
    this.on("tileunload", (event: L.TileEvent) => { this.controllers.get(event.tile)?.abort(); this.controllers.delete(event.tile); this.tiles.delete(tileKey(event.coords)); });
  }
  createTile(coordinates: L.Coords, done: L.DoneCallback): HTMLElement {
    const tile = document.createElement("canvas");
    const controller = new AbortController();
    this.controllers.set(tile, controller);
    void this.load(coordinates, window.devicePixelRatio > 1 ? 2 : 1, controller.signal).then((image) => {
      if (controller.signal.aborted) return;
      tile.width = tile.height = image.naturalWidth;
      tile.getContext("2d")!.drawImage(image, 0, 0);
      this.tiles.set(tileKey(coordinates), { coordinates, image });
      done(undefined, tile);
    }).catch((error: Error) => { if (!controller.signal.aborted) done(error, tile); });
    return tile;
  }
  async load(coordinates: L.Coords, scale: number, signal: AbortSignal) {
    const response = await fetch(`/api/heatmap/tiles/${coordinates.z}/${coordinates.x}/${coordinates.y}.png?${this.query}&scale=${scale}`, { signal, cache: "no-store", credentials: "same-origin" });
    if (response.status === 409) { this.stale(); throw new Error("Your activity history changed. Refresh the map and try again."); }
    if (!response.ok) throw new Error("Could not load heatmap tiles. Please retry.");
    const blob = await response.blob();
    const url = URL.createObjectURL(blob);
    try { const image = new Image(); image.src = url; await image.decode(); return image; }
    finally { URL.revokeObjectURL(url); }
  }
  onRemove(map: L.Map): this {
    for (const controller of this.controllers.values()) controller.abort();
    this.controllers.clear(); this.tiles.clear();
    return super.onRemove(map);
  }
}

// Anonymous loading permits export. An ordinary image retry keeps custom tile
// servers without CORS usable for viewing; canvas export detects that condition.
export class HeatmapBasemap extends L.GridLayer {
  readonly tiles = new Map<string, LoadedTile>();
  constructor(readonly url: string) {
    super({ tileSize: 256, minZoom: 0, maxZoom: 18, noWrap: true, keepBuffer: 0, updateWhenIdle: true, pane: "tilePane", attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors' });
    this.on("tileunload", (event: L.TileEvent) => this.tiles.delete(tileKey(event.coords)));
  }
  createTile(coordinates: L.Coords, done: L.DoneCallback): HTMLElement {
    const image = new Image();
    image.alt = ""; image.crossOrigin = "anonymous";
    const url = L.Util.template(this.url, { ...coordinates, s: "abc"[(coordinates.x + coordinates.y) % 3], r: "" });
    image.onload = () => { this.tiles.set(tileKey(coordinates), { coordinates, image }); done(undefined, image); };
    image.onerror = () => {
      if (image.crossOrigin) { image.removeAttribute("crossorigin"); image.src = url; }
      else done(new Error("Map background could not be loaded."), image);
    };
    image.src = url;
    return image;
  }
  onRemove(map: L.Map): this { this.tiles.clear(); return super.onRemove(map); }
}

export type HeatmapSnapshot = { center: L.LatLng; zoom: number; width: number; height: number; origin: L.Point };
export function heatmapSnapshot(map: L.Map): HeatmapSnapshot {
  const { x: width, y: height } = map.getSize();
  const center = map.getCenter(), zoom = map.getZoom();
  return { center, zoom, width, height, origin: map.project(center, zoom).subtract([width / 2, height / 2]) };
}

export async function exportHeatmap(snapshot: HeatmapSnapshot, overlay: HeatmapLayer, basemap: HeatmapBasemap, mode: "map" | "artwork", dark: boolean, caption: string, signal: AbortSignal) {
  const size = heatmapExportSize(snapshot.width, snapshot.height);
  const canvas = document.createElement("canvas"); canvas.width = size.width; canvas.height = size.height;
  const context = canvas.getContext("2d")!;
  context.fillStyle = dark ? "#101519" : "#f6f4ef";
  context.fillRect(0, 0, canvas.width, canvas.height);
  const visible: Array<{ coordinates: L.Coords; x: number; y: number }> = [];
  const count = 2 ** snapshot.zoom;
  for (let y = Math.max(0, Math.floor(snapshot.origin.y / 256)); y * 256 < snapshot.origin.y + snapshot.height && y < count; y++) {
    for (let x = Math.max(0, Math.floor(snapshot.origin.x / 256)); x * 256 < snapshot.origin.x + snapshot.width && x < count; x++) {
      visible.push({ coordinates: Object.assign(L.point(x, y), { z: snapshot.zoom }), x: (x * 256 - snapshot.origin.x) * size.scale, y: (y * 256 - snapshot.origin.y) * size.scale });
    }
  }
  if (mode === "map") {
    for (const tile of visible) {
      const loaded = basemap.tiles.get(tileKey(tile.coordinates));
      if (!loaded?.image.complete || !loaded.image.naturalWidth) throw new Error("Wait for the map background to load, or choose Artwork.");
      context.drawImage(loaded.image, tile.x, tile.y, 256 * size.scale, 256 * size.scale);
    }
    try {
      // Same grayscale/invert/brightness treatment used by the live basemap.
      const data = context.getImageData(0, 0, canvas.width, canvas.height);
      for (let i = 0; i < data.data.length; i += 4) {
        const gray = 0.2126 * data.data[i] + 0.7152 * data.data[i + 1] + 0.0722 * data.data[i + 2];
        const value = dark ? (255 - gray) * 0.72 : gray * 0.96;
        data.data[i] = data.data[i + 1] = data.data[i + 2] = value;
      }
      context.putImageData(data, 0, 0);
    } catch { throw new Error("This map provider does not allow image export. Choose Artwork to download your routes."); }
  }
  // Four at a time matches the server's bounded rendering concurrency.
  for (let i = 0; i < visible.length; i += 4) {
    await Promise.all(visible.slice(i, i + 4).map(async (tile) => {
      const image = await overlay.load(tile.coordinates, 2, signal);
      if (signal.aborted) throw new DOMException("Cancelled", "AbortError");
      context.drawImage(image, tile.x, tile.y, 256 * size.scale, 256 * size.scale);
    }));
  }
  context.fillStyle = dark ? "rgba(16,21,25,0.94)" : "rgba(246,244,239,0.94)";
  const footer = 48 * size.scale;
  context.fillRect(0, canvas.height - footer, canvas.width, footer);
  context.fillStyle = dark ? "#ecede8" : "#272c2b";
  context.font = `${12 * size.scale}px sans-serif`;
  context.fillText(`Runnarr · ${caption}`, 14 * size.scale, canvas.height - 28 * size.scale, canvas.width - 28 * size.scale);
  context.font = `${10 * size.scale}px sans-serif`;
  context.fillText(mode === "map" ? "© OpenStreetMap contributors · openstreetmap.org/copyright" : "Personal activity heatmap", 14 * size.scale, canvas.height - 11 * size.scale, canvas.width - 28 * size.scale);
  return new Promise<Blob>((resolve, reject) => canvas.toBlob((blob) => blob ? resolve(blob) : reject(new Error("Could not create PNG.")), "image/png"));
}
