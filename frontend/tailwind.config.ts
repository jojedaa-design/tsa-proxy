import type { Config } from "tailwindcss";

const config: Config = {
  content: [
    "./src/app/**/*.{js,ts,jsx,tsx,mdx}",
    "./src/lib/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        // Paleta de marca BIGDAVI
        primary: {
          50:  "#eef6ff",
          100: "#dbeafe",
          200: "#bde0fe",
          300: "#7cc3fb",
          400: "#29b6f6", // sky
          500: "#4a91f5",
          600: "#1274f2", // blue — color principal
          700: "#0f5fc9",
          800: "#0d4da3",
          900: "#0b1f3a", // navy
        },
        brand: {
          navy: "#0b1f3a",
          blue: "#1274f2",
          sky:  "#29b6f6",
        },
      },
      fontFamily: {
        heading: ["var(--font-heading)", "sans-serif"],
        sans: ["var(--font-body)", "sans-serif"],
      },
    },
  },
  plugins: [],
};

export default config;
