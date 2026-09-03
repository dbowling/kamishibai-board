import PocketBase from 'pocketbase';

/**
 * The shared PocketBase client.
 *
 * A relative base URL means the app talks to whatever origin served it: the Vite
 * dev server proxy locally, and the Go binary itself in production. There is no
 * build-time API host to get wrong.
 */
export const pb = new PocketBase(import.meta.env.BASE_URL ?? '/');

// Auto-cancellation aborts an in-flight request when an identical one starts.
// That is a sensible default for typeahead, but here it turns a quick
// double-click into a confusing "autocancelled" error, so it is off and
// concurrency is handled explicitly instead.
pb.autoCancellation(false);
