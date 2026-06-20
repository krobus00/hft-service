import type { Metadata } from "next";

import "./globals.css";

export const metadata: Metadata = {
  title: "Krobot",
  description: "Krobot dashboard",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en" suppressHydrationWarning>
      <head>
        <script
          dangerouslySetInnerHTML={{
            __html: `document.documentElement.classList.toggle("dark",localStorage.theme==="dark"||(!localStorage.theme&&matchMedia("(prefers-color-scheme: dark)").matches))`,
          }}
        />
      </head>
      <body>{children}</body>
    </html>
  );
}
