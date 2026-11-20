import React from "react";
import ReactDOM from "react-dom/client";
import { Spinner, Toast } from "@heroui/react";
import "./styles.css";

const App = React.lazy(() => import("./App").then((m) => ({ default: m.App })));
const Pay = React.lazy(() => import("./Pay").then((m) => ({ default: m.Pay })));

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <Toast.Provider placement="bottom" />
    <React.Suspense
      fallback={
        <div className="flex min-h-screen flex-col items-center justify-center gap-3 text-muted">
          <Spinner size="md" color="accent" />
          <span className="text-xs tracking-widest">正在加载 ANE</span>
        </div>
      }
    >
      {location.pathname.startsWith("/pay/") ? <Pay /> : <App />}
    </React.Suspense>
  </React.StrictMode>,
);
