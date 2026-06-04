/** @type {import("next").NextConfig} */
const nextConfig = {
  output: "standalone",
  typescript: {
    ignoreBuildErrors: true,
  },
  eslint: {
    ignoreDuringBuilds: true,
  },
  async rewrites() {
    return [
      {
        source: "/api/admin/:path*",
        destination: `${process.env.BACKEND_URL || "http://backend:8080"}/api/admin/:path*`,
      },
    ];
  },
};

export default nextConfig;
