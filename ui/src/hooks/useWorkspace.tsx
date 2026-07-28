import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from 'react';
import { api } from '@/api/client';
import type { WorkspaceContext } from '@/api/types';
import { useAuth } from '@/hooks/useAuth';

type WorkspaceContextValue = {
  workspace: WorkspaceContext | null;
  loading: boolean;
  refresh: () => Promise<void>;
  switchWorkspace: (mspID: string, clientID?: string, siteID?: string) => Promise<void>;
};

const WorkspaceState = createContext<WorkspaceContextValue | null>(null);

export function WorkspaceProvider({ children }: { children: ReactNode }) {
  const { user } = useAuth();
  const [workspace, setWorkspace] = useState<WorkspaceContext | null>(null);
  const [loading, setLoading] = useState(false);

  const refresh = useCallback(async () => {
    if (!user) {
      setWorkspace(null);
      return;
    }
    setLoading(true);
    try {
      setWorkspace(await api.getWorkspaceContext());
    } finally {
      setLoading(false);
    }
  }, [user]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const switchWorkspace = async (mspID: string, clientID = '', siteID = '') => {
    const result = await api.switchWorkspace(mspID, clientID, siteID);
    api.setToken(result.token);
    await refresh();
  };

  return (
    <WorkspaceState.Provider value={{ workspace, loading, refresh, switchWorkspace }}>
      {children}
    </WorkspaceState.Provider>
  );
}

export function useWorkspace() {
  const context = useContext(WorkspaceState);
  if (!context) throw new Error('useWorkspace must be used within WorkspaceProvider');
  return context;
}
