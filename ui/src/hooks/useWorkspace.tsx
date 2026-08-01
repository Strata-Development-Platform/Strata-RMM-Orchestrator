import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from 'react';
import { api } from '@/api/client';
import type { ProviderBusinessProfile, WorkspaceContext } from '@/api/types';
import { useAuth } from '@/hooks/useAuth';

type WorkspaceContextValue = {
  workspace: WorkspaceContext | null;
  loading: boolean;
  error: string | null;
  refresh: () => Promise<void>;
  applyProviderProfile: (profile: ProviderBusinessProfile) => void;
  switchWorkspace: (mspID: string, clientID?: string, siteID?: string) => Promise<void>;
};

const WorkspaceState = createContext<WorkspaceContextValue | null>(null);

export function WorkspaceProvider({ children }: { children: ReactNode }) {
  const { user } = useAuth();
  const [workspace, setWorkspace] = useState<WorkspaceContext | null>(null);
  const [loadedUserID, setLoadedUserID] = useState('');
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    if (!user) {
      setWorkspace(null);
      setLoadedUserID('');
      setError(null);
      return;
    }
    setError(null);
    try {
      setWorkspace(await api.getWorkspaceContext());
      setLoadedUserID(user.user_id);
    } catch (caught) {
      setWorkspace(null);
      setLoadedUserID(user.user_id);
      setError(caught instanceof Error ? caught.message : 'Workspace context unavailable');
      throw caught;
    }
  }, [user]);

  useEffect(() => {
    void refresh().catch(() => undefined);
  }, [refresh]);

  const switchWorkspace = async (mspID: string, clientID = '', siteID = '') => {
    const result = await api.switchWorkspace(mspID, clientID, siteID);
    api.setToken(result.token);
    await refresh();
  };

  const applyProviderProfile = (profile: ProviderBusinessProfile) => {
    setWorkspace(current => current ? {
      ...current,
      provider_display_name: profile.display_name,
      setup_complete: profile.setup_complete,
    } : current);
  };

  return (
    <WorkspaceState.Provider value={{
      workspace,
      loading: Boolean(user && loadedUserID !== user.user_id),
      error,
      refresh,
      applyProviderProfile,
      switchWorkspace,
    }}>
      {children}
    </WorkspaceState.Provider>
  );
}

export function useWorkspace() {
  const context = useContext(WorkspaceState);
  if (!context) throw new Error('useWorkspace must be used within WorkspaceProvider');
  return context;
}
