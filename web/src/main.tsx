import React from "react";
import ReactDOM from "react-dom/client";
import "./styles.css";
const App = React.lazy(() => import("./App").then((m) => ({ default: m.App })));
const Pay = React.lazy(() => import("./Pay").then((m) => ({ default: m.Pay })));
ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <React.Suspense
      fallback={<div className="p-10 text-center text-muted">正在加载 ANE</div>}
    >
      {location.pathname.startsWith("/pay/") ? <Pay /> : <App />}
    </React.Suspense>
  </React.StrictMode>,
);
