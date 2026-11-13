import { useEffect, useState } from "react";
import {
  MapContainer,
  TileLayer,
  CircleMarker,
  Circle,
  Popup,
  useMap,
  useMapEvents,
} from "react-leaflet";
import { Link } from "react-router-dom";
import type { Location } from "../api/types";
import "leaflet/dist/leaflet.css";
export type MapItem = { id: string; name: string; location: Location };
export const latLng = (p: Location): [number, number] => [
  p.latitude,
  p.longitude,
];
function Controller({
  center,
  onPick,
}: {
  center: Location;
  onPick?: (p: Location) => void;
}) {
  const map = useMap();
  useEffect(() => {
    map.setView(latLng(center));
    const timer = setTimeout(() => map.invalidateSize(), 100);
    return () => clearTimeout(timer);
  }, [center.latitude, center.longitude, map]);
  useMapEvents({
    click: (e) => onPick?.({ longitude: e.latlng.lng, latitude: e.latlng.lat }),
  });
  return null;
}
export default function MapCanvas({
  items = [],
  center = { longitude: 71.4304, latitude: 51.1282 },
  radius,
  onPick,
}: {
  items?: MapItem[];
  center?: Location;
  radius?: number;
  onPick?: (p: Location) => void;
}) {
  const [tileError, setTileError] = useState(false);
  const tileURL =
    import.meta.env.VITE_MAP_TILE_URL ||
    "https://tile.openstreetmap.org/{z}/{x}/{y}.png";
  return (
    <div className="map-wrap">
      {tileError && (
        <div className="map-warning" role="status">
          Подложка недоступна. Список и ручной ввод координат работают.
        </div>
      )}
      <MapContainer
        center={latLng(center)}
        zoom={13}
        scrollWheelZoom={false}
        className="map-canvas"
      >
        <TileLayer
          url={tileURL}
          attribution={
            import.meta.env.VITE_MAP_ATTRIBUTION ||
            '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors'
          }
          eventHandlers={{ tileerror: () => setTileError(true) }}
        />
        <Controller center={center} onPick={onPick} />
        {radius && (
          <Circle
            center={latLng(center)}
            radius={radius}
            pathOptions={{ color: "#287b68", fillOpacity: 0.06, weight: 1 }}
          />
        )}
        {onPick && (
          <CircleMarker
            center={latLng(center)}
            radius={8}
            pathOptions={{ color: "#b66528", fillOpacity: 1 }}
          />
        )}
        {items.map((p) => (
          <CircleMarker
            key={p.id}
            center={latLng(p.location)}
            radius={9}
            pathOptions={{
              color: "#fff",
              weight: 2,
              fillColor: "#17604f",
              fillOpacity: 1,
            }}
          >
            <Popup>
              <strong>{p.name}</strong>
              <br />
              <Link to={"/places/" + p.id}>Подробнее →</Link>
            </Popup>
          </CircleMarker>
        ))}
      </MapContainer>
    </div>
  );
}
