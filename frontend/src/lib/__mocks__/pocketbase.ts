// Storybook's stand-in for the PocketBase client (see .storybook/preview.tsx).
//
// It keeps a real, observable auth store so AuthProvider behaves as in the app,
// and refuses everything else: data access is stubbed at the `api` layer by
// src/stories/fakeBackend.ts, so any call that reaches here is a story that
// forgot to stub something and should fail loudly.
type Listener = (token: string, record: unknown) => void;

let token = '';
let record: Record<string, unknown> | null = null;
const listeners = new Set<Listener>();
const emit = () => listeners.forEach((listener) => listener(token, record));

const authStore = {
  get record() {
    return record;
  },
  get token() {
    return token;
  },
  get isValid() {
    return record !== null;
  },
  save(nextToken: string, next: Record<string, unknown> | null) {
    token = nextToken;
    record = next;
    emit();
  },
  clear() {
    token = '';
    record = null;
    emit();
  },
  onChange(listener: Listener) {
    listeners.add(listener);
    return () => {
      listeners.delete(listener);
    };
  },
};

const notMocked = (what: string) => () =>
  Promise.reject(
    new Error(`${what} is not available in Storybook; stub it via src/stories/fakeBackend.ts`),
  );

export const pb = {
  authStore,
  autoCancellation(_enabled: boolean) {},
  filter: (raw: string, params?: Record<string, unknown>) => `${raw} ${JSON.stringify(params ?? {})}`,
  send: notMocked('pb.send'),
  collection(name: string) {
    return {
      subscribe: async () => async () => {},
      // Any password except "wrong-password" signs in, so the login stories can
      // show both outcomes. An address starting "admin" gets the admin role.
      async authWithPassword(email: string, password: string) {
        if (password === 'wrong-password') {
          throw { response: { message: 'Failed to authenticate.' } };
        }
        const user = email.startsWith('admin')
          ? { id: 'uadmin', name: 'Avery Admin', email, role: 'admin' }
          : { id: 'udana', name: 'Dana Ops', email, role: 'user' };
        authStore.save('storybook-token', user);
        return { token: 'storybook-token', record: user };
      },
      getFullList: notMocked(`${name}.getFullList`),
      create: notMocked(`${name}.create`),
      update: notMocked(`${name}.update`),
    };
  },
};

export default pb;
