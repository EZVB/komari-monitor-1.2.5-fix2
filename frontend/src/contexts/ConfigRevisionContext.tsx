import React from "react";
import { useRPC2Call } from "./RPC2Context";

type ConfigRevisions = {
  nodes: number;
  settings: number;
};

const ConfigRevisionContext = React.createContext<ConfigRevisions | null>(null);

export const ConfigRevisionProvider: React.FC<{
  children: React.ReactNode;
}> = ({ children }) => {
  const { call } = useRPC2Call();
  const [revisions, setRevisions] = React.useState<ConfigRevisions | null>(null);
  const requestPendingRef = React.useRef(false);

  const poll = React.useCallback(async () => {
    if (requestPendingRef.current || document.hidden) return;
    requestPendingRef.current = true;
    try {
      const next = await call<Record<string, never>, ConfigRevisions>(
        "common:getConfigRevisions",
      );
      if (
        next &&
        Number.isFinite(next.nodes) &&
        Number.isFinite(next.settings)
      ) {
        setRevisions((current) =>
          current?.nodes === next.nodes && current?.settings === next.settings
            ? current
            : next,
        );
      }
    } catch {
      // The normal data providers surface connection errors; this poll is best effort.
    } finally {
      requestPendingRef.current = false;
    }
  }, [call]);

  React.useEffect(() => {
    void poll();
    const interval = window.setInterval(() => void poll(), 2000);
    const handleVisibilityChange = () => {
      if (!document.hidden) void poll();
    };
    document.addEventListener("visibilitychange", handleVisibilityChange);
    return () => {
      window.clearInterval(interval);
      document.removeEventListener("visibilitychange", handleVisibilityChange);
    };
  }, [poll]);

  return (
    <ConfigRevisionContext.Provider value={revisions}>
      {children}
    </ConfigRevisionContext.Provider>
  );
};

export const useConfigRevisions = () =>
  React.useContext(ConfigRevisionContext);
