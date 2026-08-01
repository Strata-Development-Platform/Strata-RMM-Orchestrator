import { createContext, useContext, useState, useEffect, type ReactNode } from 'react';
import { api } from '@/api/client';
import type { LoginResponse } from '@/api/types';

type AuthContextType = {
  user: LoginResponse | null;
  loading: boolean;
  login: (email: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  refreshSession: () => Promise<void>;
};

const AuthContext = createContext<AuthContextType | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<LoginResponse | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    if (api.getToken()) {
      api.me()
        .then(setUser)
        .catch(() => api.setToken(null))
        .finally(() => setLoading(false));
    } else {
      setLoading(false);
    }
  }, []);

  const login = async (email: string, password: string) => {
    const res = await api.login(email, password);
    api.setToken(res.token);
    setUser(res);
  };

  const logout = async () => {
    try {
      await api.logout();
    } finally {
      api.setToken(null);
      setUser(null);
    }
  };

  const refreshSession = async () => {
    setUser(await api.me());
  };

  return (
    <AuthContext.Provider value={{ user, loading, login, logout, refreshSession }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth must be used within AuthProvider');
  return ctx;
}
