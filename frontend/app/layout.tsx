import type { Metadata } from "next";
import { Inter, Space_Mono } from "next/font/google";
import "./globals.css";
import { Web3Providers } from "@/components/Web3Providers";

const inter = Inter({
  variable: "--font-inter",
  subsets: ["latin"],
  fallback: ["Inter Fallback", "sans-serif"],
});

const spaceMono = Space_Mono({
  variable: "--font-space-mono",
  weight: ["400", "700"],
  subsets: ["latin"],
});

export const metadata: Metadata = {
  metadataBase: new URL("https://meta-max.xyz"),
  title: "MetaMax - Trustless Agentic Cloud",
  description: "Every cloud provider asks you to trust them. We're the only one that proves you can't.",
};

// Every page mounts the wallet providers, and WalletConnect reaches for
// localStorage as soon as it initialises — which does not exist during static
// prerendering, and fails the production build. Nothing here is cacheable
// anyway: the whole app is per-wallet, live chain state.
export const dynamic = "force-dynamic";

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html
      lang="en"
      className={`${inter.variable} ${spaceMono.variable} h-full antialiased`}
    >
      <body className="min-h-full flex flex-col">
        <Web3Providers>{children}</Web3Providers>
      </body>
    </html>
  );
}
