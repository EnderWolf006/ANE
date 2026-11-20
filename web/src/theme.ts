import { useCallback, useEffect, useState } from "react";

export type ThemeMode = "light" | "dark" | "system";
export type ResolvedTheme = "light" | "dark";

const STORAGE_KEY = "ane-theme";
const THEME_COLORS: Record<ResolvedTheme, string> = {
  light: "#156d51",
  dark: "#0d1412",
};

function getSystemTheme(): ResolvedTheme {
  if (typeof window === "undefined") return "light";
  return window.matchMedia("(prefers-color-scheme: dark)").matches
    ? "dark"
    : "light";
}

export function getStoredTheme(): ThemeMode {
  if (typeof window === "undefined") return "system";
  try {
    const stored = window.localStorage.getItem(STORAGE_KEY);
    if (stored === "light" || stored === "dark" || stored === "system") {
      return stored;
    }
  } catch {
    // ignore storage access errors
  }
  return "system";
}

export function applyTheme(mode: ThemeMode): ResolvedTheme {
  const resolved: ResolvedTheme = mode === "system" ? getSystemTheme() : mode;
  if (typeof document !== "undefined") {
    const root = document.documentElement;
    root.classList.toggle("dark", resolved === "dark");
    root.classList.toggle("light", resolved === "light");
    root.setAttribute("data-theme", resolved);
    root.style.colorScheme = resolved;
    const meta = document.querySelector('meta[name="theme-color"]');
    if (meta) meta.setAttribute("content", THEME_COLORS[resolved]);
  }
  return resolved;
}

export function useThemeMode() {
  const [mode, setModeState] = useState<ThemeMode>(getStoredTheme);
  const [resolved, setResolved] = useState<ResolvedTheme>(() =>
    applyTheme(getStoredTheme()),
  );

  const setMode = useCallback((next: ThemeMode) => {
    setModeState(next);
    try {
      window.localStorage.setItem(STORAGE_KEY, next);
    } catch {
      // ignore storage access errors
    }
    setResolved(applyTheme(next));
  }, []);

  const cycleMode = useCallback(() => {
    setMode(resolved === "dark" ? "light" : "dark");
  }, [resolved, setMode]);

  useEffect(() => {
    setResolved(applyTheme(mode));
    if (mode !== "system") return;
    const media = window.matchMedia("(prefers-color-scheme: dark)");
    const listener = () => setResolved(applyTheme("system"));
    media.addEventListener("change", listener);
    return () => media.removeEventListener("change", listener);
  }, [mode]);

  return { mode, resolved, setMode, cycleMode };
}
