import { Link, useLocation } from "react-router-dom";
import {
  ArrowUpRight,
  Coffee,
  Trees,
  BookOpen,
  Building2,
  Landmark,
  MapPin,
} from "lucide-react";
import type { Place } from "../../../shared/api/types";
import { categoryNames, distance } from "../../../shared/lib/format";
const icons = {
  cafe: Coffee,
  park: Trees,
  library: BookOpen,
  coworking: Building2,
  attraction: Landmark,
};
export function PlaceCard({
  place,
  children,
}: {
  place: Place;
  children?: React.ReactNode;
}) {
  const location = useLocation();
  const Icon = icons[place.category] ?? MapPin;
  return (
    <article className="place-card">
      <div className={"category-icon " + place.category}>
        <Icon size={25} />
      </div>
      <div className="place-card-body">
        <div className="card-meta">
          <span>{categoryNames[place.category]}</span>
          {place.distance_m != null && (
            <span>{distance(place.distance_m)}</span>
          )}
        </div>
        <Link
          className="place-title"
          to={"/places/" + place.id}
          state={{ from: location.pathname + location.search }}
        >
          {place.name}
          <ArrowUpRight size={18} />
        </Link>
        <p className="address">
          <MapPin size={13} />
          {place.address}
        </p>
        <div className="tags">
          {place.tags.map((tag) => (
            <span key={tag}>{tag}</span>
          ))}
        </div>
        {children}
      </div>
    </article>
  );
}
