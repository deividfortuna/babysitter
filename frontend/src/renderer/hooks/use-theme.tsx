import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useInsertionEffect,
  useMemo,
  useState,
  useSyncExternalStore,
  type ReactNode,
} from "react";
import { bridge } from "@/lib/bridge";
import {
  applyTheme,
  readThemePreference,
  storeThemePreference,
  systemTheme,
  watchSystemTheme,
  type Theme,
  type ThemePreference,
} from "@/lib/theme";

type ThemeContextValue = {
  preference: ThemePreference;
  theme: Theme;
  setPreference: (preference: ThemePreference) => void;
};

const ThemeContext = createContext<ThemeContextValue | null>(null);

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [preference, setPreference] = useState(readThemePreference);
  const system = useSyncExternalStore(watchSystemTheme, systemTheme);
  const theme = preference === "system" ? system : preference;

  useInsertionEffect(() => applyTheme(theme), [theme]);
  useEffect(() => bridge.theme.follow(preference), [preference]);

  const choose = useCallback((next: ThemePreference) => {
    storeThemePreference(next);
    setPreference(next);
  }, []);

  const value = useMemo(() => ({ preference, theme, setPreference: choose }), [preference, theme, choose]);

  return <ThemeContext value={value}>{children}</ThemeContext>;
}

export function useTheme(): ThemeContextValue {
  const value = useContext(ThemeContext);
  if (!value) throw new Error("useTheme needs a ThemeProvider above it");
  return value;
}
