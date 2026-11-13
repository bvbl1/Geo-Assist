import {
  createContext,
  useContext,
  useEffect,
  useRef,
  type ReactNode,
} from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { identity } from "../modules/identity/api";
import { ApiError, resetRequests } from "../shared/api/client";
import type { User } from "../shared/api/types";
type Session = {
  user: User | null | undefined;
  loading: boolean;
  error: Error | null;
  reload: () => void;
  accept: (user: User | null) => void;
};
const Context = createContext<Session | null>(null);
export function SessionProvider({ children }: { children: ReactNode }) {
  const client = useQueryClient();
  const channel = useRef<BroadcastChannel | null>(null);
  const query = useQuery({
    queryKey: ["session"],
    staleTime: 10000,
    retry: false,
    queryFn: async ({ signal }) => {
      try {
        return await identity.me(signal);
      } catch (e) {
        if (e instanceof ApiError && e.status === 401) return null;
        throw e;
      }
    },
  });
  const previous = useRef<string | null | undefined>(undefined);
  useEffect(() => {
    const id = query.data?.id ?? (query.data === null ? null : undefined);
    if (
      id !== undefined &&
      previous.current !== undefined &&
      previous.current !== id
    ) {
      resetRequests();
      client.removeQueries({ predicate: (q) => q.queryKey[0] === "user" });
    }
    previous.current = id;
  }, [query.data, client]);
  useEffect(() => {
    const expire = () => {
      resetRequests();
      client.removeQueries({ predicate: (q) => q.queryKey[0] === "user" });
      client.setQueryData(["session"], null);
    };
    window.addEventListener("session-expired", expire);
    if ("BroadcastChannel" in window) {
      channel.current = new BroadcastChannel("geoassist-session");
      channel.current.onmessage = () => {
        resetRequests();
        client.removeQueries({ queryKey: ["user"] });
        void client.resetQueries({ queryKey: ["session"] });
      };
    }
    return () => {
      window.removeEventListener("session-expired", expire);
      channel.current?.close();
    };
  }, [client]);
  function accept(user: User | null) {
    resetRequests();
    client.removeQueries({ queryKey: ["user"] });
    client.setQueryData(["session"], user);
    previous.current = user?.id ?? null;
    channel.current?.postMessage("changed");
  }
  return (
    <Context.Provider
      value={{
        user: query.data,
        loading: query.isPending,
        error: query.error,
        reload: () => {
          void query.refetch();
        },
        accept,
      }}
    >
      {children}
    </Context.Provider>
  );
}
export function useSession() {
  const value = useContext(Context);
  if (!value) throw new Error("Missing session provider");
  return value;
}
