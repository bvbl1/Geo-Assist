import type { components } from "./generated/schema";
export type User = components["schemas"]["User"];
export type Place = components["schemas"]["Place"];
export type Category = components["schemas"]["Category"];
export type State = components["schemas"]["State"];
export type Favorite = components["schemas"]["Favorite"];
export type Interaction = components["schemas"]["Interaction"];
export type Location = components["schemas"]["Coordinates"];
export type Page<T> = {
  data: T[];
  pagination: { limit: number; offset: number; has_more: boolean };
};
export type Data<T> = { data: T };
export type PlaceCreate = components["schemas"]["PlaceCreate"];
export type PlacePatch = components["schemas"]["PlacePatch"];
