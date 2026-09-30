import { useCallback, useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useSearchParams } from "react-router-dom";
import { MapContainer } from "react-leaflet";
import L from "leaflet";
import { Download, Maximize2, Minimize2, Scan, X } from "lucide-react";
import { api } from "./api";
import type { ThemePreference } from "./theme";
import { heatmapAppearance, heatmapCaption, heatmapDatePreset, heatmapQuery, heatmapViewport } from "./heatmap";
import type { HeatmapAppearance, HeatmapMetadata } from "./heatmap";
import { exportHeatmap, HeatmapBasemap, HeatmapLayer, heatmapSnapshot } from "./heatmapMap";
import type { HeatmapSnapshot } from "./heatmapMap";

type ExportSelection = { snapshot: HeatmapSnapshot; overlay: HeatmapLayer; basemap: HeatmapBasemap; dark: boolean; caption: string; revision: number; query: string };

export function HeatmapPage({ accountID, theme, tileURL }: { accountID: string; theme: ThemePreference; tileURL?: string }) {
  const [params, setParams] = useSearchParams();
  const currentParams = useRef(params); currentParams.current = params;
  const initialViewport = useRef(heatmapViewport(params));
  const fitted = useRef(Boolean(initialViewport.current));
  const [map, setMap] = useState<L.Map | null>(null);
  const [systemDark, setSystemDark] = useState(() => window.matchMedia("(prefers-color-scheme: dark)").matches);
  const [fullscreen, setFullscreen] = useState(false);
  const [ready, setReady] = useState(false);
  const [tileError, setTileError] = useState("");
  const [reload, setReload] = useState(0);
  const [selection, setSelection] = useState<ExportSelection>();
  const layers = useRef<{ overlay?: HeatmapLayer; basemap?: HeatmapBasemap }>({});
  const previous = useRef<{ data: HeatmapMetadata; query: string }>();
  const query = heatmapQuery(params);
  const appearance = (["app", "light", "dark"].includes(params.get("appearance") ?? "") ? params.get("appearance") : "app") as HeatmapAppearance;
  const resolved = heatmapAppearance(appearance, theme, systemDark);
  const metadata = useQuery({ queryKey: ["heatmap", accountID, query], queryFn: ({ signal }) => api.heatmap(query, signal), refetchInterval: (q) => q.state.data?.pending ? 2000 : 15000 });
  if (metadata.data) previous.current = { data: metadata.data, query };
  const displayed = metadata.data ? { data: metadata.data, query } : previous.current;
  const data = displayed?.data;
  const filteredReady = Boolean(metadata.data && !metadata.data.pending && metadata.data.mapped && !metadata.isFetching && !metadata.error);
  const editParams = useCallback((edit: (next: URLSearchParams) => void) => {
    const next = new URLSearchParams(currentParams.current); edit(next); currentParams.current = next; setParams(next, { replace: true });
  }, [setParams]);
  const fit = useCallback(() => {
    if (map && data?.bounds) { const [w, s, e, n] = data.bounds; map.fitBounds([[s, w], [n, e]], { padding: [28, 28], maxZoom: 15, animate: false }); }
  }, [map, data]);

  useEffect(() => {
    const media = window.matchMedia("(prefers-color-scheme: dark)");
    const change = () => setSystemDark(media.matches); media.addEventListener("change", change);
    return () => media.removeEventListener("change", change);
  }, []);
  useEffect(() => {
    if (!map) return;
    const save = () => { const center = map.getCenter(); editParams((next) => { next.set("lat", center.lat.toFixed(6)); next.set("lng", Math.max(-180, Math.min(180, center.lng)).toFixed(6)); next.set("zoom", String(map.getZoom())); }); };
    map.on("moveend", save);
    return () => { map.off("moveend", save); };
  }, [map, editParams]);
  useEffect(() => {
    if (map && data?.bounds && !data.pending && !fitted.current) { fitted.current = true; fit(); }
  }, [map, data, fit]);
  useEffect(() => {
    if (!map) return;
    const url = !tileURL || tileURL.includes("tile.openstreetmap.org") ? "https://tile.openstreetmap.org/{z}/{x}/{y}.png" : tileURL;
    const base = new HeatmapBasemap(url); layers.current.basemap = base; base.addTo(map);
    return () => { base.remove(); layers.current.basemap = undefined; };
  }, [map, tileURL]);
  useEffect(() => {
    if (!map) return;
    const pane = map.getPane("tilePane");
    if (pane) pane.style.filter = resolved === "dark" ? "grayscale(1) invert(1) brightness(0.72)" : "grayscale(1) brightness(0.96)";
  }, [map, resolved]);
  useEffect(() => {
    if (!map || !displayed) return;
    setReady(false); setTileError("");
    const tileQuery = `${displayed.query}&revision=${displayed.data.revision}&appearance=${resolved}`;
    let stale = false, failed = false;
    const layer = new HeatmapLayer(tileQuery, () => { if (!stale) { stale = true; void metadata.refetch(); } });
    layers.current.overlay = layer;
    layer.on("loading", () => { setReady(false); });
    layer.on("tileerror", () => { failed = true; setTileError("Some routes could not be loaded. Retry to see the complete heatmap."); setReady(false); });
    layer.on("load", () => { setReady(!failed); });
    layer.addTo(map);
    return () => { layer.remove(); layers.current.overlay = undefined; };
  }, [map, displayed?.query, displayed?.data.revision, resolved, reload]);
  useEffect(() => {
    const timeout = window.setTimeout(() => map?.invalidateSize({ pan: false }), 0);
    if (!fullscreen) return () => window.clearTimeout(timeout);
    const key = (event: KeyboardEvent) => { if (event.key === "Escape") setFullscreen(false); };
    const overflow = document.body.style.overflow; document.body.style.overflow = "hidden";
    window.addEventListener("keydown", key);
    return () => { window.clearTimeout(timeout); document.body.style.overflow = overflow; window.removeEventListener("keydown", key); };
  }, [fullscreen, map]);
  const retry = () => { setReload((value) => value + 1); void metadata.refetch(); };
  const openExport = () => {
    const { overlay, basemap } = layers.current;
    if (map && overlay && basemap && metadata.data) setSelection({ snapshot: heatmapSnapshot(map), overlay, basemap, dark: resolved === "dark", caption: heatmapCaption(params), revision: metadata.data.revision, query });
  };
  const selectPreset = (preset: string) => editParams((next) => {
    const range = heatmapDatePreset(preset);
    for (const key of ["from", "to"] as const) { if (range[key]) next.set(key, range[key]); else next.delete(key); }
  });
  const sports = params.getAll("sport");
  const selectedSports = sports.length ? sports : ["running"];

  return <div className="page heatmap-page">
    <header className="page-header"><div><div className="eyebrow">Your routes over time</div><h1>Heatmap</h1></div></header>
    <section className="panel heatmap-filters" aria-label="Heatmap filters">
      <div className="heatmap-presets" aria-label="Date presets">
        <button className="secondary-button small-button" onClick={() => selectPreset("all")}>All time</button>
        <button className="secondary-button small-button" onClick={() => selectPreset("year")}>This year</button>
        <button className="secondary-button small-button" onClick={() => selectPreset("last12")}>Last 12 months</button>
      </div>
      <div className="heatmap-filter-fields">
        <label className="field"><span>From</span><input type="date" value={params.get("from") ?? ""} max={params.get("to") || undefined} onChange={(event) => editParams((next) => event.target.value ? next.set("from", event.target.value) : next.delete("from"))} /></label>
        <label className="field"><span>To</span><input type="date" value={params.get("to") ?? ""} min={params.get("from") || undefined} onChange={(event) => editParams((next) => event.target.value ? next.set("to", event.target.value) : next.delete("to"))} /></label>
        <label className="field heatmap-sports"><span>Sports</span><select multiple aria-label="Sports" value={selectedSports} onChange={(event) => { const values = Array.from(event.target.selectedOptions, (option) => option.value); const newest = values.find((value) => !selectedSports.includes(value)); const chosen = newest === "all" || newest === "running" ? [newest] : values.filter((value) => value !== "all" && value !== "running"); editParams((next) => { next.delete("sport"); for (const sport of chosen.length ? chosen : ["running"]) next.append("sport", sport); }); }}><option value="running">Running</option><option value="all">All sports</option>{data?.sports.map((sport) => <option key={sport} value={sport}>{sport === "Run" ? "Run (recorded type)" : sport}</option>)}</select></label>
        <label className="field"><span>Appearance</span><select value={appearance} onChange={(event) => editParams((next) => next.set("appearance", event.target.value))}><option value="app">App theme</option><option value="light">Light</option><option value="dark">Dark</option></select></label>
      </div>
    </section>
    <div className="heatmap-summary" role="status" aria-live="polite">
      {data && <><strong>{data.mapped.toLocaleString()} mapped activities</strong><span>{(data.distanceM / 1000).toLocaleString(undefined, { maximumFractionDigits: 1 })} km in selection</span>{data.excluded > 0 && <span>{data.excluded.toLocaleString()} without usable GPS</span>}</>}
      {metadata.isPending && <span>Loading activity history…</span>}
      {Boolean(data?.pending) && <span>Preparing routes: {((data?.mapped ?? 0) + (data?.excluded ?? 0)).toLocaleString()} of {((data?.mapped ?? 0) + (data?.excluded ?? 0) + (data?.pending ?? 0)).toLocaleString()} activities</span>}
      {!metadata.isPending && metadata.isFetching && <span>Updating…</span>}
    </div>
    {metadata.error && <div className="error">{metadata.error.message} <button className="secondary-button small-button" onClick={retry}>Retry</button></div>}
    {metadata.data && !metadata.data.pending && !metadata.data.mapped && <div className="panel heatmap-empty">{metadata.data.excluded ? "These activities have no usable GPS routes. Choose another sport or date range." : "No activities match these filters. Try a wider date range or another sport."}</div>}
    <section className={`panel heatmap-map-panel ${fullscreen ? "heatmap-fullscreen" : ""}`} data-appearance={resolved} aria-label="Personal activity heatmap">
      <div className="heatmap-map-toolbar"><div><strong>{heatmapCaption(params)}</strong><span className="muted">Each activity counts once at a location.</span></div><div className="heatmap-map-actions">
        <button className="secondary-button small-button" onClick={fit} disabled={!data?.bounds}><Scan size={15} />Fit routes</button>
        <button className="secondary-button small-button" onClick={() => setFullscreen(!fullscreen)} aria-pressed={fullscreen}>{fullscreen ? <Minimize2 size={15} /> : <Maximize2 size={15} />}{fullscreen ? "Exit fullscreen" : "Fullscreen"}</button>
        <button className="primary-button small-button" onClick={openExport} disabled={!filteredReady || !ready || Boolean(tileError)}><Download size={15} />Download PNG</button>
      </div></div>
      <MapContainer ref={setMap} center={initialViewport.current?.center ?? [53.35, -6.26]} zoom={initialViewport.current?.zoom ?? 12} minZoom={0} maxZoom={18} className="heatmap-map" scrollWheelZoom worldCopyJump={false} maxBounds={[[-85.051129, -180], [85.051129, 180]]} maxBoundsViscosity={1} />
      <div className="heatmap-legend" aria-label="Activity frequency legend"><span>Activities</span><div><div className="heatmap-gradient" /><div className="heatmap-legend-ticks">{[1, 2, 5, 10, 20, 50, 100].map((value) => <span key={value} style={{ left: `${Math.log1p(value) / Math.log(101) * 100}%` }}>{value === 100 ? "100+" : value}</span>)}</div></div></div>
      {tileError && <div className="error">{tileError} <button className="secondary-button small-button" onClick={retry}>Retry</button></div>}
    </section>
    {selection && <HeatmapExportDialog selection={selection} onClose={() => setSelection(undefined)} />}
  </div>;
}

function HeatmapExportDialog({ selection, onClose }: { selection: ExportSelection; onClose: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null);
  const [mode, setMode] = useState<"map" | "artwork">("map");
  const [preview, setPreview] = useState("");
  const [error, setError] = useState("");
  const [checking, setChecking] = useState(false);
  const [attempt, setAttempt] = useState(0);
  useEffect(() => { dialog.current?.showModal(); }, []);
  useEffect(() => {
    const controller = new AbortController(); let url = "";
    setPreview(""); setError("");
    void exportHeatmap(selection.snapshot, selection.overlay, selection.basemap, mode, selection.dark, selection.caption, controller.signal)
      .then((blob) => { if (!controller.signal.aborted) { url = URL.createObjectURL(blob); setPreview(url); } })
      .catch((reason: Error) => { if (!controller.signal.aborted) setError(reason.message); });
    return () => { controller.abort(); if (url) URL.revokeObjectURL(url); };
  }, [selection, mode, attempt]);
  const download = async () => {
    setChecking(true);
    try {
      const current = await api.heatmap(selection.query);
      if (current.revision !== selection.revision) throw new Error("Your activity history changed. Close this dialog and export the refreshed map.");
      const link = document.createElement("a"); link.href = preview; link.download = `runnarr-heatmap-${mode}.png`; link.click();
    } catch (reason) { setError(reason instanceof Error ? reason.message : "Could not verify this export."); }
    finally { setChecking(false); }
  };
  return <dialog ref={dialog} className="heatmap-export-dialog" aria-labelledby="heatmap-export-title" onCancel={onClose}>
    <div className="heatmap-map-toolbar"><h2 id="heatmap-export-title">Download heatmap</h2><button className="icon-button" aria-label="Close export" onClick={onClose}><X size={18} /></button></div>
    <fieldset className="heatmap-export-modes"><legend>Image background</legend><label><input type="radio" name="heatmap-export-mode" checked={mode === "map"} onChange={() => setMode("map")} />Map</label><label><input type="radio" name="heatmap-export-mode" checked={mode === "artwork"} onChange={() => setMode("artwork")} />Artwork</label></fieldset>
    {preview ? <img className="heatmap-export-preview" src={preview} alt={`${mode === "map" ? "Map" : "Artwork"} export preview`} /> : !error && <p role="status">Preparing your image…</p>}
    {error && <div className="error" role="alert">{error} <button className="secondary-button small-button" onClick={() => setAttempt((value) => value + 1)}>Retry export</button></div>}
    <div className="heatmap-map-actions"><button className="secondary-button" onClick={onClose}>Close</button><button className="primary-button" disabled={!preview || Boolean(error) || checking} onClick={() => void download()}><Download size={16} />{checking ? "Checking…" : "Save PNG"}</button></div>
  </dialog>;
}
