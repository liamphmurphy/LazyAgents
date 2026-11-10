import type { NextConfig } from "next";

const apiInternal = process.env.API_INTERNAL_URL ?? "http://localhost:8000";

const nextConfig: NextConfig = {
  output: "standalone",
  cacheComponents: true,
  partialPrefetching: true,
  turbopack: {
    rules: {
      "*.css": {
        loaders: ["@tailwindcss/turbopack"],
        as: "*.css",
      },
    },
  },
  // Memory and Personas became one page; the query (?id=) carries over.
  async redirects() {
    return [
      { source: "/memory", destination: "/customization", permanent: false },
      { source: "/personas", destination: "/customization", permanent: false },
      // Agents sit beside Personas in Customization; /agents/:handle stays the editor.
      { source: "/agents", destination: "/customization?who=agents", permanent: false },
      // Board and Epics are layouts of Tickets; their filters (?project=, ?epic=) carry over.
      { source: "/board", destination: "/tickets?layout=board", permanent: false },
      { source: "/epics", destination: "/tickets?layout=epics", permanent: false },
      { source: "/costs", destination: "/runs?tab=costs", permanent: false },
    ];
  },
  // In compose, Caddy routes these paths to the API before they reach Next.
  // These rewrites make `next dev` work on its own without Caddy.
  async rewrites() {
    return [
      { source: "/api/:path*", destination: `${apiInternal}/api/:path*` },
      { source: "/mcp", destination: `${apiInternal}/mcp` },
      { source: "/events", destination: `${apiInternal}/events` },
    ];
  },
};

export default nextConfig;
