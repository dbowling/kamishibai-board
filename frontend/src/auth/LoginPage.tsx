import { useState } from 'react';
import type { FormEvent } from 'react';
import { useAuth } from './AuthProvider';
import { errorMessage } from '../lib/api';

export function LoginPage() {
  const { signIn } = useAuth();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function onSubmit(event: FormEvent) {
    event.preventDefault();
    setError(null);
    setBusy(true);

    try {
      await signIn(email, password);
    } catch (cause) {
      // Deliberately not distinguishing "no such account" from "wrong password".
      setError(errorMessage(cause, 'Those credentials were not accepted.'));
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="login">
      <form className="login__card" onSubmit={onSubmit} aria-labelledby="login-heading">
        <h1 id="login-heading" className="login__title">
          Kamishibai
          <span className="login__subtitle">Triage board</span>
        </h1>

        <label className="field">
          <span className="field__label">Email</span>
          <input
            className="field__input"
            type="email"
            name="email"
            autoComplete="username"
            required
            value={email}
            onChange={(e) => setEmail(e.target.value)}
          />
        </label>

        <label className="field">
          <span className="field__label">Password</span>
          <input
            className="field__input"
            type="password"
            name="password"
            autoComplete="current-password"
            required
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </label>

        {/* role=alert so a screen reader announces the failure without needing focus moved */}
        {error && (
          <p className="login__error" role="alert">
            {error}
          </p>
        )}

        <button className="button button--primary" type="submit" disabled={busy}>
          {busy ? 'Signing in…' : 'Sign in'}
        </button>

        <p className="login__hint">
          Accounts are created by an administrator. There is no self-service signup.
        </p>
      </form>
    </main>
  );
}
