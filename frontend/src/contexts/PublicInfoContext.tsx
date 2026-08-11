import React from "react";
import defaultTheme from "../../komari-theme.json";
import { useConfigRevisions } from "./ConfigRevisionContext";
//import { useRPC2Call } from "./RPC2Context";

type ThemeField = {
  key?: string;
  default?: unknown;
};

const defaultThemeSettings = Object.fromEntries(
  (
    (defaultTheme.configuration?.data ?? []) as ThemeField[]
  )
    .filter(
      (field) =>
        typeof field.key === "string" &&
        Object.prototype.hasOwnProperty.call(field, "default"),
    )
    .map((field) => [field.key, field.default]),
);

const withThemeDefaults = (publicInfo: PublicInfo): PublicInfo => {
  if (publicInfo.theme !== "default") {
    return publicInfo;
  }

  return {
    ...publicInfo,
    theme_settings: {
      ...defaultThemeSettings,
      ...(publicInfo.theme_settings ?? {}),
    },
  };
};

export interface PublicInfo {
  cors_origin_check_enabled: boolean;
  custom_body: string;
  custom_head: string;
  description: string;
  metric_retention_days: number;
  sitename: string;
  private_site: boolean;
  theme: string;
  theme_settings: any;
  [property: string]: any;
}

interface Response {
  data: PublicInfo;
  message: string;
  status: string;
  [property: string]: any;
}

interface PublicInfoContextType {
  publicInfo: PublicInfo | null;
  isLoading: boolean;
  error: string | null;
  refresh: () => void;
}

const PublicInfoContext = React.createContext<PublicInfoContextType | undefined>(
  undefined
);

export const PublicInfoProvider: React.FC<{ children: React.ReactNode }> = ({
  children,
}) => {
  const [publicInfo, setPublicInfo] = React.useState<PublicInfo | null>(null);
  const [isLoading, setIsLoading] = React.useState<boolean>(false);
  const [error, setError] = React.useState<string | null>(null);
  const revisions = useConfigRevisions();
  const refreshSeqRef = React.useRef(0);
  const mountedRef = React.useRef(true);
  const observedRevisionRef = React.useRef<number | null>(null);
  //const { call } = useRPC2Call();
  // 公共信息使用public，避免在私有站点的情况下RPC返回401
  const refresh = React.useCallback(() => {
    const refreshSeq = ++refreshSeqRef.current;
    setError(null);
    setIsLoading(true);
    fetch("/api/public")
      .then((response) => {
        if (!response.ok) {
          throw new Error("Failed to fetch public info");
        }
        return response.json();
      })
      .then((resp: Response) => {
        if (!mountedRef.current || refreshSeq !== refreshSeqRef.current) return;
        if (resp && resp.data) {
          setPublicInfo(withThemeDefaults(resp.data));
        } else {
          setPublicInfo(null);
        }
      })
      .catch((err) => {
        if (!mountedRef.current || refreshSeq !== refreshSeqRef.current) return;
        setError(err.message || "An error occurred while fetching public info");
      })
      .finally(() => {
        if (!mountedRef.current || refreshSeq !== refreshSeqRef.current) return;
        setIsLoading(false);
      });
  }, []);

  React.useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  React.useEffect(() => {
    refresh();
  }, [refresh]);

  React.useEffect(() => {
    if (revisions === null) return;
    if (observedRevisionRef.current === null) {
      observedRevisionRef.current = revisions.settings;
      return;
    }
    if (observedRevisionRef.current !== revisions.settings) {
      observedRevisionRef.current = revisions.settings;
      refresh();
    }
  }, [refresh, revisions]);

  return (
    <PublicInfoContext.Provider value={{ publicInfo, isLoading, error, refresh }}>
      {children}
    </PublicInfoContext.Provider>
  );
};

export const usePublicInfo = () => {
  const context = React.useContext(PublicInfoContext);
  if (!context) {
    throw new Error("usePublicInfo must be used within a PublicInfoProvider");
  }
  return context;
};
