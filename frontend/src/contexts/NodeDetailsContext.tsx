import React from 'react';

export type NodeDetail = {
  uuid: string;
  token: string;
  name: string;
  cpu_name: string;
  virtualization: string;
  arch: string;
  cpu_cores: number;
  os: string;
  gpu_name: string;
  ipv4: string;
  ipv6: string;
  region: string;
  mem_total: number;
  swap_total: number;
  disk_total: number;
  version: string;
  weight: number;
  price: number;
  currency: string;
  auto_renewal: boolean;
  remark: string | undefined;
  public_remark: string;
  group: string | undefined;
  billing_cycle: number;
  traffic_limit: number;
  traffic_limit_type: "sum" | "max" | "min" | "up" | "down";
  traffic_multiplier: number;
  traffic_reset_day: number;
  traffic_initial: number;
  traffic_initial_at: string | null;
  traffic_used: number;
  traffic_cycle_start: string | null;
  expired_at: string;
  created_at: string;
  updated_at: string;
  [key: string]: any; 
};

interface NodeDetailsContextType {
  nodeDetail: NodeDetail[] | [];
  isLoading: boolean;
  error: string | null;
  refresh: () => void;
}
const NodeDetailsContext = React.createContext<NodeDetailsContextType | undefined>(undefined);
export const NodeDetailsProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [nodeDetail, setNodeDetail] = React.useState<NodeDetail[] | []>([]);
  const [isLoading, setIsLoading] = React.useState<boolean>(false);
  const [error, setError] = React.useState<string | null>(null);

  const refresh = () => {
    fetch("/api/admin/client/list")
      .then((response) => response.json())
      .then((data: NodeDetail[]) => {
        setNodeDetail(data);
        setIsLoading(false);
      })
      .catch((error) => {
        setError(error.message);
        setIsLoading(false);
      });
  };
    React.useEffect(() => {
        setIsLoading(true);
        refresh();
    }, []);
  return (
    <NodeDetailsContext.Provider value={{ nodeDetail, isLoading, error, refresh }}>
      {children}
    </NodeDetailsContext.Provider>
  );
};

export const useNodeDetails = () => {
    const context = React.useContext(NodeDetailsContext);
    if (context === undefined) {
        throw new Error("useNodeDetails must be used within a NodeDetailsProvider");
    }
    return context;
};
