import type { MetadataRoute } from "next";
export default function manifest(): MetadataRoute.Manifest {
  return { name: "Git/Me developer workspace", short_name: "Git/Me", description: "Your personal developer workstation.", start_url: "/", display: "standalone", background_color: "#ffffff", theme_color: "#3981F4", icons: [{ src: "/icon.svg", sizes: "any", type: "image/svg+xml", purpose: "any" }] };
}
