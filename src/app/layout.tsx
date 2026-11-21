import type { Metadata, Viewport } from "next";
import Script from "next/script";
import { CompanionProvider } from "@/components/CompanionConnection";
import { WorkspacePersistence } from "@/components/WorkspaceStorage";
import { GitHubProvider } from "@/components/GitHubConnection";
import { VercelProvider } from "@/components/VercelConnection";
import "./globals.css";
import "./theme.css";
import "./notes.css";
import "./scrollbars.css";
import "./task-board.css";
import "./projects.css";
import "./checklists.css";
import "./services.css";
import "./companion.css";
import "./storage.css";
import "./project-scan.css";
import "./runtime.css";
import "./github.css";
import "./vercel.css";
import "./integration-actions.css";
import "./releases.css";
import "./activity.css";
import "./local-git.css";
import "./dropdowns.css";
import "./api-tester.css";

export const metadata: Metadata = {
  title: "Git/Me | Developer workspace",
  description: "Your local projects, tools and workflows in one workspace.",
  applicationName: "Git/Me",
  icons: { icon: "/icon.svg", apple: "/icon.svg" },
};
export const viewport: Viewport = { themeColor: [{ media: "(prefers-color-scheme: light)", color: "#ffffff" }, { media: "(prefers-color-scheme: dark)", color: "#101010" }] };

const themeScript = `(function(){var t;try{t=localStorage.getItem('gitme.theme')}catch(e){}document.documentElement.dataset.theme=t==='dark'||(t!=='light'&&matchMedia('(prefers-color-scheme: dark)').matches)?'dark':'light'})()`;

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en" suppressHydrationWarning>
      <body>
        <Script id="gitme-theme" strategy="beforeInteractive">{themeScript}</Script>
        <CompanionProvider><GitHubProvider><VercelProvider><WorkspacePersistence/>{children}</VercelProvider></GitHubProvider></CompanionProvider>
      </body>
    </html>
  );
}
