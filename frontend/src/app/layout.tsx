import type { Metadata } from "next";
import { Inter, Space_Grotesk } from "next/font/google";
import "./globals.css";
import { Providers } from "./providers";

// Nota: la tipografía de marca (Eurostile Extd / Avenir LT Pro) es comercial
// y no está disponible como web font. Space Grotesk + Inter son el reemplazo
// más cercano disponible (geométrica extendida / humanista para UI).
const spaceGrotesk = Space_Grotesk({
  subsets: ["latin"],
  weight: ["500", "600", "700"],
  variable: "--font-heading",
});

const inter = Inter({
  subsets: ["latin"],
  weight: ["400", "500", "600", "700", "800"],
  variable: "--font-body",
});

export const metadata: Metadata = {
  title: "BIGDAVI | Panel de Administración TSA",
  description: "Panel de administración del sello de tiempo (RFC 3161) — BIGDAVI",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="es" className={`${spaceGrotesk.variable} ${inter.variable}`}>
      <body className="bg-gray-50 text-gray-900 antialiased font-sans">
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
