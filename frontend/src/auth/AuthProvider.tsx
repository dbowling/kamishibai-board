import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import type { ReactNode } from 'react';
import { browserTimeZone } from '../lib/format';
import { pb } from '../lib/pocketbase';
import type { CurrentUser } from '../lib/types';

interface AuthContextValue {
  user: CurrentUser | null;
  isAdmin: boolean;
  signIn: (email: string, password: string) => Promise<void>;
  signOut: () => void;
}

const AuthContext = createContext<AuthContextValue | null>(null);

function toCurrentUser(record: unknown): CurrentUser | null {
  if (!record || typeof record !== 'object') return null;
  const r = record as Record<string, unknown>;
  if (typeof r.id !== 'string') return null;

  return {
    id: r.id,
    name: typeof r.name === 'string' ? r.name : '',
    email: typeof r.email === 'string' ? r.email : '',
    // Anything other than 'admin' is an ordinary user, matching the backend's
    // fail-closed treatment of an absent role.
    role: r.role === 'admin' ? 'admin' : 'user',
    timezone: typeof r.timezone === 'string' ? r.timezone : '',
  };
}

export function AuthProvider({ children }: { children: ReactNode }) {
  // PocketBase persists the auth store in localStorage, so a refresh keeps the
  // session without a round trip.
  const [user, setUser] = useState<CurrentUser | null>(() => toCurrentUser(pb.authStore.record));

  useEffect(() => {
    // Fires on sign-in, sign-out, and when a token is refreshed or rejected.
    const unsubscribe = pb.authStore.onChange(() => {
      setUser(toCurrentUser(pb.authStore.record));
    });
    return unsubscribe;
  }, []);

  const signIn = useCallback(async (email: string, password: string) => {
    await pb.collection('users').authWithPassword(email, password);
  }, []);

  const signOut = useCallback(() => {
    pb.authStore.clear();
  }, []);

  const value = useMemo<AuthContextValue>(
    () => ({ user, isAdmin: user?.role === 'admin', signIn, signOut }),
    [user, signIn, signOut],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextValue {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error('useAuth must be used inside an AuthProvider');
  }
  return context;
}

/**
 * The zone to render instants in: the signed-in user's own choice, or the
 * browser's when they have not made one.
 *
 * Display only. It must never feed anything that decides which period a card is
 * in, because that is the team's zone and the server's call. Unlike useAuth it
 * does not throw outside an AuthProvider, so components that merely format a time
 * can still be rendered on their own.
 */
export function useDisplayTimeZone(): string {
  const user = useContext(AuthContext)?.user;
  return user?.timezone || browserTimeZone();
}
