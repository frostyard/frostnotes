import "./styles/app.css";

import { suppressNativeMenu } from "./app/system";

// The web build (vite build --mode web) runs inside lodge's shell and
// connects through it (docs/design/app.md#in-a-browser); every other build
// is the desktop app's or the development browser's. Each branch's
// imports stay out of the other's bundle.
const root = document.getElementById("root");
if (!root) throw new Error("missing #root");
suppressNativeMenu();
if (import.meta.env.MODE === "web") {
  const { runInLodge } = await import("./lodge/main");
  await runInLodge(root);
} else {
  const { runDesktop } = await import("./desktop");
  await runDesktop(root);
}
