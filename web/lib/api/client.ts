import createClient from "openapi-fetch";
import type { components, paths } from "./schema";

// The browser always talks to the same origin (Caddy in compose, Next
// rewrites in dev). Server components call the API over the internal network.
const baseUrl =
  typeof window === "undefined"
    ? `${process.env.API_INTERNAL_URL ?? "http://localhost:8000"}/api/v1`
    : "/api/v1";

export const api = createClient<paths>({ baseUrl });

export type Schemas = components["schemas"];
