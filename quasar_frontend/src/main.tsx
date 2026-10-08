import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";
import ReactDOM from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import { AppRouter } from "./app/AppRouter";
import { ThemeProvider } from "./app/ThemeProvider";
import "./styles/themes.css";
import "./styles/global.css";
import "./styles/responsive.css";
import "./styles/accent-skin.css";
import "./styles/mobile-skin.css";
import "./styles/login.css";
import "./styles/pwa.css";
import { registerServiceWorker } from "./pwa/registerServiceWorker";
import "./pwa/install"; // registra o ouvinte de beforeinstallprompt já na abertura do app

const qc = new QueryClient({
  defaultOptions: {
    queries: { retry: 1, refetchOnWindowFocus: false },
  },
});

registerServiceWorker();

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <QueryClientProvider client={qc}>
      <ThemeProvider>
        <BrowserRouter>
          <AppRouter />
        </BrowserRouter>
      </ThemeProvider>
    </QueryClientProvider>
  </React.StrictMode>,
);
