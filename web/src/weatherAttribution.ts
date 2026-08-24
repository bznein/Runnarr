import type { ActivityWeather } from "./types";

const multiModelMethod = "midpoint-15-minute-multi-model";
const archiveMethod = "midpoint-nearest-hour-archive";

export function openMeteoUIAttribution(weather: ActivityWeather) {
  if (weather.selectionMethod === multiModelMethod) {
    const selectedModel = weather.model?.trim() ? ` (${weather.model.trim()})` : "";
    return `Model-derived at the activity midpoint from 15-minute values. The median-temperature model among UKMO, ICON, and ECMWF supplied this record${selectedModel}. WMO code rendered as text.`;
  }
  if (weather.selectionMethod === archiveMethod) {
    return "Model-derived at the activity midpoint from the nearest-hour archive value. WMO code rendered as text.";
  }
  return "Model-derived conditions. WMO code rendered as text.";
}

export function openMeteoAISource(weather: ActivityWeather) {
  if (weather.selectionMethod === multiModelMethod) {
    const selectedModel = weather.model?.trim() ? `: ${weather.model.trim()}` : "";
    return `Open-Meteo (model-derived at the activity midpoint from 15-minute values; median-temperature model selected from UKMO, ICON, and ECMWF${selectedModel}; WMO code rendered as text; CC BY 4.0): https://open-meteo.com/`;
  }
  if (weather.selectionMethod === archiveMethod) {
    return "Open-Meteo (model-derived at the activity midpoint from the nearest-hour archive value; WMO code rendered as text; CC BY 4.0): https://open-meteo.com/";
  }
  return "Open-Meteo (model-derived; WMO code rendered as text; CC BY 4.0): https://open-meteo.com/";
}
