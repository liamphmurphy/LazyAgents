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
