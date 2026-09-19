"use client";

import {
  createContext,
  useContext,
  useEffect,
  useCallback,
  useState,
  useRef,
  type ReactNode,
} from "react";
import { useAccount, useSignMessage } from "wagmi";

const API =
  typeof window !== "undefined"
    ? "/api/backend"
    : (process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080");

function isTokenValid(token: string | undefined): boolean {
  if (!token) return false;
  if (token.includes(".") && token.split(".").length === 3) {
    try {
      const payload = token.split(".")[1];
      const decoded = JSON.parse(atob(payload.replace(/-/g, "+").replace(/_/g, "/")));
      if (!decoded.exp) return true;
      return decoded.exp * 1000 > Date.now();
    } catch {
      return false;
    }
  }
  const parts = token.split("|");
  if (parts.length === 3) {
    const exp = parseInt(parts[1], 10);
    return !isNaN(exp) && exp * 1000 > Date.now();
  }
  return false;
}

export const STORAGE = {
  JWT:        "comput3_jwt",
  WALLET:     "comput3_wallet",
  TEAM_ID:    "comput3_team_id",
  TEAM_NAME:  "comput3_team_name",
  WORKSPACES: "comput3_workspaces",
} as const;

type AuthState = {
  address: string | undefined;
  isConnected: boolean;
  token: string | undefined;
  isAuthenticated: boolean;
  isAuthenticating: boolean;
  hydrated: boolean;
  teamId: string | undefined;
  teamName: string | undefined;
  isNewAccount: boolean;
  authenticate: () => Promise<void>;
  setTeam: (id: string, name?: string) => void;
  addWorkspace: (containerId: string) => void;
  logout: () => void;
};

const DEFAULT: AuthState = {
  address: undefined,
  isConnected: false,
  token: undefined,
  isAuthenticated: false,
  isAuthenticating: false,
  hydrated: false,
  teamId: undefined,
  teamName: undefined,
  isNewAccount: false,
  authenticate: async () => {},
  setTeam: () => {},
  addWorkspace: () => {},
  logout: () => {},
};

const AuthContext = createContext<AuthState>(DEFAULT);

export function AuthProvider({ children }: { children: ReactNode }) {
  const { address, isConnected } = useAccount();
  const { signMessageAsync } = useSignMessage();
  const authInFlight = useRef(false);
  const autoAuthTriggeredFor = useRef<string>("");

  const [token, setTokenState] = useState<string | undefined>(() => {
    if (typeof window === "undefined") return undefined;
    const t = localStorage.getItem(STORAGE.JWT) ?? undefined;
    return isTokenValid(t) ? t : undefined;
  });

  const [teamId, setTeamIdState] = useState<string | undefined>(() => {
    if (typeof window === "undefined") return undefined;
    return localStorage.getItem(STORAGE.TEAM_ID) ?? undefined;
  });

  const [teamName, setTeamNameState] = useState<string | undefined>(() => {
    if (typeof window === "undefined") return undefined;
    return localStorage.getItem(STORAGE.TEAM_NAME) ?? undefined;
  });

  const [isAuthenticating, setIsAuthenticating] = useState(false);
  const [isNewAccount, setIsNewAccount] = useState(false);
  const [hydrated, setHydrated] = useState(false);

  useEffect(() => { setHydrated(true); }, []);

  const isAuthenticated = isTokenValid(token);

  const fetchAccount = useCallback(async (wallet: string) => {
    try {
      const res = await fetch(`${API}/account?wallet=${encodeURIComponent(wallet)}`);
      if (!res.ok) return;
      const account = await res.json();
      const defaultName = !account.name || account.name.startsWith("account-");
      localStorage.setItem(STORAGE.TEAM_ID, account.id);
      localStorage.setItem(STORAGE.TEAM_NAME, account.name ?? "");
      setTeamIdState(account.id);
      setTeamNameState(account.name);
      setIsNewAccount(!!defaultName);
    } catch {
      // Non-fatal
    }
  }, []);

  const setTeam = useCallback((id: string, name?: string) => {
    localStorage.setItem(STORAGE.TEAM_ID, id);
    if (name) localStorage.setItem(STORAGE.TEAM_NAME, name);
    setTeamIdState(id);
    if (name) setTeamNameState(name);
  }, []);

  const addWorkspace = useCallback((containerId: string) => {
    const hist: string[] = JSON.parse(localStorage.getItem(STORAGE.WORKSPACES) ?? "[]");
    localStorage.setItem(STORAGE.WORKSPACES, JSON.stringify([containerId, ...hist].slice(0, 20)));
  }, []);

  const logout = useCallback(() => {
    localStorage.removeItem(STORAGE.JWT);
    localStorage.removeItem(STORAGE.WALLET);
    localStorage.removeItem(STORAGE.TEAM_ID);
    localStorage.removeItem(STORAGE.TEAM_NAME);
    setTokenState(undefined);
    setTeamIdState(undefined);
    setTeamNameState(undefined);
    setIsNewAccount(false);
    autoAuthTriggeredFor.current = "";
  }, []);

  const authenticate = useCallback(async () => {
    if (!address || authInFlight.current) return;
    authInFlight.current = true;
    setIsAuthenticating(true);
    localStorage.setItem(STORAGE.WALLET, address.toLowerCase());
    try {
      const nonceRes = await fetch(`${API}/auth/nonce`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ address }),
      });
      if (!nonceRes.ok) return;
      const { nonce } = await nonceRes.json();

      const signature = await signMessageAsync({ message: nonce });

      const verifyRes = await fetch(`${API}/auth/verify`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ address, nonce, signature }),
      });
      if (!verifyRes.ok) return;
      const { token: t } = await verifyRes.json();

      localStorage.setItem(STORAGE.JWT, t);
      localStorage.setItem(STORAGE.WALLET, address.toLowerCase());
      setTokenState(t);

      await fetchAccount(address.toLowerCase());
    } catch {
      // Wallet rejection or network error — silently ignore
    } finally {
      authInFlight.current = false;
      setIsAuthenticating(false);
    }
  }, [address, signMessageAsync, fetchAccount]);

  useEffect(() => {
    if (!isConnected || !address) {
      autoAuthTriggeredFor.current = "";
      return;
    }
    const storedWallet = localStorage.getItem(STORAGE.WALLET);
    const walletMatches = storedWallet === address.toLowerCase();
    const storedToken = localStorage.getItem(STORAGE.JWT);
    const storedTeamId = localStorage.getItem(STORAGE.TEAM_ID);

    if (!walletMatches || !isTokenValid(storedToken ?? undefined)) {
      if (autoAuthTriggeredFor.current !== address.toLowerCase()) {
        autoAuthTriggeredFor.current = address.toLowerCase();
        authenticate();
      }
    } else if (storedToken && !token) {
      setTokenState(storedToken);
      if (!storedTeamId) {
        fetchAccount(address.toLowerCase());
      }
    }
  }, [isConnected, address]); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (!isConnected) {
      setTokenState(undefined);
    }
  }, [isConnected]);

  return (
    <AuthContext.Provider
      value={{
        address,
        isConnected,
        token,
        isAuthenticated,
        isAuthenticating,
        hydrated,
        teamId,
        teamName,
        isNewAccount,
        authenticate,
        setTeam,
        addWorkspace,
        logout,
      }}
    >
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth(): AuthState {
  return useContext(AuthContext);
}
