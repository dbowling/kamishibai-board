# Realtime

How a teammate's click reaches your screen.

This document assumes you have never built a realtime application. It starts from
the problem and works up to the code, so if you already know what server-sent
events are you can skip to [How PocketBase does it](#how-pocketbase-does-it) or
[Our implementation](#our-implementation).

## The problem

The requirement is that all team members share the same board state. Five people
have the board open. Dana marks "Verify Backups" as done.

If nothing else happens, the other four are now looking at a board that is wrong.
Raj sees "Verify Backups" as not started, picks it up, and spends ten minutes
re-checking backups that Dana already checked. The board's whole purpose — making
the state of the routine visible at a glance — is defeated by it being stale.

So the four other browsers need to find out. There are three ways to arrange that.

### Option 1: make the user press refresh

The state is correct only immediately after a manual reload. In practice people stop
trusting the board and start asking in chat instead, which is the thing the board was
supposed to replace.

### Option 2: polling

Ask the server every few seconds whether anything changed.

```ts
// Not what we do.
setInterval(() => refetchBoard(), 5000);
```

This works, and for many applications it is a perfectly reasonable choice. Its
problems here are:

- **Latency you can feel.** With a five-second interval, the average delay is 2.5
  seconds and the worst case is five. Long enough for Raj to have already started.
- **Almost all the requests are wasted.** A triage board changes a handful of times
  a day. At five seconds, each open tab makes about 17,000 requests a day, and
  essentially all of them return "nothing changed".
- **Cost scales with tabs, not with activity.** Ten people leaving the board open
  overnight generate load all night for nothing.

Polling makes the client responsible for asking. What we actually want is for the
server to tell us, because the server is the thing that knows.

### Option 3: the server pushes (what we do)

The browser opens **one long-lived connection** and holds it open. When something
changes, the server writes a message down that connection. No polling, and the delay
is however long the network takes.

## Server-sent events

The transport is **server-sent events** (SSE), a plain HTTP feature that has been in
browsers for well over a decade.

An ordinary HTTP request works like this: the browser asks, the server answers, the
connection closes. SSE changes one thing — **the server never finishes the
response**. It sends a header saying `Content-Type: text/event-stream`, then writes a
line of text whenever it has something to say, potentially for hours.

From the browser's point of view it is a request that is permanently still loading.
That is why it shows as pending in the network tab, and why that is normal rather
than a hung request.

### Why not WebSockets?

WebSockets are the better-known option, and they are the right tool when the client
also needs to send a stream of messages — a chat app, a collaborative editor, a game.

Here, traffic is almost entirely one-directional: the server tells the client things
happened, and the client's own actions go through ordinary REST calls. For that
shape, SSE is simpler:

| | SSE | WebSockets |
| --- | --- | --- |
| Protocol | Plain HTTP | Upgrade handshake to `ws://` |
| Direction | Server to client | Both |
| Reconnects | Automatic, built into the browser | You implement it |
| Proxies, CDNs, TLS | Just HTTP | Often needs specific configuration |
| Auth headers | Same as any request | Awkward; often needs a query param |

The automatic reconnection matters more than it sounds. A laptop lid closing, a wifi
handover, or a rolling deploy all break the connection, and with SSE the browser
retries on its own.

PocketBase chose SSE, so that is what we use.

## How PocketBase does it

Two endpoints and three steps. Understanding this is worth the ten minutes, because
it makes debugging obvious later.

```
1. Browser:  GET /api/realtime                     ← opens the stream, stays open
   Server:   event:PB_CONNECT  data:{"clientId":"abc123"}

2. Browser:  POST /api/realtime                    ← "here is what I care about"
             { "clientId": "abc123",
               "subscriptions": ["occurrences"] }

3. Server:   event:occurrences  data:{"action":"update","record":{...}}
             ... whenever an occurrence changes, down the connection from step 1
```

The reason for two endpoints is that the stream from step 1 is one-directional. The
client cannot talk back down it, so it needs a separate ordinary request to say what
it wants. The `clientId` from step 1 is what ties the two together.

### Seeing it for yourself

This is the single most useful thing to do if you want realtime to stop feeling like
magic. Start the backend, then in one terminal:

```bash
curl -N http://localhost:8090/api/realtime
```

`-N` disables curl's buffering, so you see lines as they arrive. Almost immediately:

```
id:5aq0cztqhpp1n2b
event:PB_CONNECT
data:{"clientId":"5aq0cztqhpp1n2b"}
```

The command does not exit. That is the point.

Now, in a second terminal, register a subscription using that `clientId`. Occurrences
are only visible to team members, so get a token first:

```bash
TOKEN=$(curl -s -X POST http://localhost:8090/api/collections/users/auth-with-password \
  -H 'content-type: application/json' \
  -d '{"identity":"dana@example.test","password":"kamishibai-dev-1234"}' \
  | jq -r .token)

curl -s -X POST http://localhost:8090/api/realtime \
  -H 'content-type: application/json' \
  -H "Authorization: $TOKEN" \
  -d '{"clientId":"5aq0cztqhpp1n2b","subscriptions":["occurrences"]}'
```

Then complete a card:

```bash
CARD=$(curl -s -H "Authorization: $TOKEN" \
  'http://localhost:8090/api/collections/cards/records?perPage=1&filter=cadence="daily"' \
  | jq -r '.items[0].id')

curl -s -X POST -H "Authorization: $TOKEN" \
  "http://localhost:8090/api/kamishibai/cards/$CARD/complete" \
  -H 'content-type: application/json' -d '{"notes":"from curl"}' >/dev/null
```

The first terminal prints, unprompted:

```
id:5aq0cztqhpp1n2b
event:occurrences
data:{"action":"create","record":{"id":"...","card":"...","board":"...","team":"...","cadence":"daily","period_key":"2026-09-03","status":"done","started_by":"...","completed_by":"...","notes":"from curl", ...}}
```

That is the entire mechanism. Everything below is convenience on top of it.

### The message format

SSE frames are three lines and a blank line:

```
id:<clientId>
event:<subscription topic>
data:<JSON payload>

```

For record changes the payload is always:

```json
{
  "action": "create" | "update" | "delete",
  "record": { ...the full record... }
}
```

Two things to note about `record`:

- It is the record **as that subscriber is allowed to see it** — hidden fields
  removed, and relations not expanded unless the subscription asked for it.
- On `delete` it is the record as it was just before deletion.

### Subscription topics

| Topic | Meaning |
| --- | --- |
| `occurrences` | Every change in the collection |
| `occurrences/RECORD_ID` | One specific record |
| `occurrences/*?filter=...` | Collection-wide, filtered server-side |

## Is this secure?

Yes, and this is worth being precise about, because "we opened a firehose of database
changes to the browser" is a reasonable thing to be nervous about.

Before delivering any event, PocketBase evaluates the collection's access rule
**for that specific subscriber**, using that subscriber's own auth state:

```go
// apis/realtime.go, simplified
requestInfo := &core.RequestInfo{
	Context: core.RequestInfoContextRealtime,
	Auth:    clientAuth,       // this subscriber's token
}
if !realtimeCanAccessRecord(accessCheckApp, record, requestInfo, rule) {
	continue                  // deliver nothing to this client
}
```

Which rule depends on the topic:

| Subscription | Rule checked |
| --- | --- |
| Collection-wide (`occurrences`, `occurrences/*`) | the collection's **ListRule** |
| A single record (`occurrences/abc123`) | the collection's **ViewRule** |

For `occurrences`, both are `TeamMember`:

```
@request.auth.id != "" && (@request.auth.role = "admin" || team.members.id ?= @request.auth.id)
```

So subscribing to the whole collection gets you events for **your teams only**. Sam,
who is on Security, never sees an event about a Platform Engineering card. An
unauthenticated subscriber sees nothing at all. This is the same rule that governs
the REST API, evaluated by the same code, so there is no second permission model that
could drift out of step with the first.

It also means **broadening a collection's ListRule broadens its realtime stream**. If
you ever loosen a rule, remember you are loosening two things.

Worth knowing: events fire on database writes regardless of *who* wrote them,
including our own server-side code. That is exactly what makes this design work.
Clients cannot write to `occurrences` at all — every mutation goes through
`/api/kamishibai/cards/:id/complete`, which writes the row server-side — and the
resulting write still broadcasts to every subscriber. Locking clients out of the
table does not cost us the live updates.

See [Permissions](permissions.md) for the full picture.

## The SDK

The PocketBase JS SDK handles the whole dance: opening the stream, catching the
`clientId`, POSTing the subscription list, reconnecting, and re-registering
subscriptions after a reconnect.

```ts
const unsubscribe = await pb.collection('occurrences').subscribe('*', (event) => {
  console.log(event.action);   // 'create' | 'update' | 'delete'
  console.log(event.record);   // the record
});

// later
unsubscribe();
```

`subscribe` returns a **promise of an unsubscribe function**. The promise is the part
people trip over: subscribing is an async network operation, so you get a promise,
and the function you must eventually call is inside it. See
[Pitfalls](#pitfalls-and-how-we-avoid-them).

The SDK also multiplexes. Ten `subscribe` calls share **one** SSE connection; the
list of topics is just re-POSTed. So subscribing in several components is cheap and
does not open several connections.

## Our implementation

All of it is in `frontend/src/boards/useBoardState.ts`.

```ts
// Realtime: refetch when any occurrence on this board changes.
useEffect(() => {
  if (!boardId) return;

  let disposed = false;
  let unsubscribe: (() => void) | undefined;

  pb.collection('occurrences')
    .subscribe('*', (event) => {
      const record = event.record as unknown as { board?: string } | undefined;
      // Occurrences carry a denormalised board id, so filtering is a field read
      // rather than a lookup.
      if (record?.board === boardId) {
        refresh();
      }
    })
    .then((dispose) => {
      if (disposed) {
        void dispose();
        return;
      }
      unsubscribe = dispose;
    })
    .catch(() => {
      // A failed subscription degrades to manual refresh, which is not worth
      // interrupting the user over.
    });

  return () => {
    disposed = true;
    if (unsubscribe) void unsubscribe();
  };
}, [boardId, refresh]);
```

Four decisions in there are worth explaining.

### Why we subscribe to occurrences and nothing else

`occurrences` is the only collection that changes during normal use. Cards, boards
and teams are edited occasionally; occurrences change every time anybody does any
work. Subscribing to the one high-churn collection covers the case that actually
matters.

### Why we filter by board on the client

An event arrives for every occurrence on every team you belong to, but the screen is
showing one board. The filter is a single field comparison, because `board` is
denormalised onto each occurrence row precisely so that checks like this need no
lookup:

```ts
if (record?.board === boardId) refresh();
```

The alternative is a server-side filter, which the SDK supports:

```ts
// Fewer wakeups, one more thing to keep in sync.
pb.collection('occurrences').subscribe('*', handler, {
  filter: pb.filter('board = {:board}', { board: boardId }),
});
```

That is a genuine improvement at scale, since the server stops sending events you
will discard. We did not do it because a team's traffic is a handful of events per
hour, and a client-side `if` is one line with nothing to get wrong. If a team grows
to the point where irrelevant events are noticeable, the server-side filter is the
change to make.

### Why we refetch instead of patching state

The event contains the whole updated record. It is tempting to splice it into local
state and skip the network:

```ts
// Tempting, and wrong here.
setState((prev) => patchCard(prev, event.record));
```

We deliberately refetch the board instead. The reasons:

- **An occurrence is not a card.** The board renders cards with their state, plus
  attribution names, plus a live tally. Reconstructing all of that from one
  occurrence row means reimplementing part of the `/state` endpoint in the browser,
  and keeping the two in agreement forever.
- **Events can be missed.** If the connection drops for thirty seconds, the events
  during that window are gone. A client patching incrementally is now quietly wrong
  until something forces a full reload. A client that refetches converges on the
  next event no matter what it missed.
- **The server's view is the one that counts.** Something else may have changed in
  the same moment — another card, an archived card, a period rolling over.

The cost is one extra request per change. For a board that changes a few times an
hour, that is nothing, and it buys correctness that is hard to get any other way.

This is the right trade *here*. For something like a collaborative text editor, where
changes arrive many times a second, refetching would be absurd and patching is the
only option.

### Why the `disposed` flag

This handles a real race. `subscribe` is async, so the component can unmount before
the promise resolves:

```
t=0   effect runs, subscribe() starts
t=1   user navigates away, cleanup runs, unsubscribe is still undefined
t=2   subscribe() resolves — and we are now subscribed with nobody to clean it up
```

That is a leaked subscription: a handler holding a stale `boardId`, firing forever.
The flag closes the gap by unsubscribing immediately if cleanup already happened:

```ts
.then((dispose) => {
  if (disposed) { void dispose(); return; }
  unsubscribe = dispose;
})
```

Any async subscription in React needs this shape.

## What realtime does not cover

**A period rolling over produces no event.**

This follows directly from the lazy model. When Monday arrives, weekly cards become
not-started because the period key changed — **no row was written**. No write means no
database event means nothing to broadcast. There is nothing PocketBase could tell you
about, because from its point of view nothing happened.

So the client schedules its own refresh, using boundaries the server already gave it:

```ts
useEffect(() => {
  if (!state) return;

  const periods = Object.values(state.periods).filter((p) => p !== undefined);
  const ms = msUntilNextBoundary(periods);
  if (ms === null) return;

  // A second of slack so the server has definitely crossed the boundary.
  const timer = window.setTimeout(refresh, ms + 1_000);
  return () => window.clearTimeout(timer);
}, [state, refresh]);
```

A board left open overnight flips by itself in the morning.

The two mechanisms are complementary, and it is worth holding the distinction: **the
realtime stream reports things people did; the timer reports time passing.**

## Pitfalls and how we avoid them

### Forgetting to unsubscribe

The classic leak. Every subscription must be cleaned up in the effect's return, or
navigating between boards accumulates handlers, each holding an old `boardId` and
each triggering a refetch.

### The async unmount race

Covered above. Without the `disposed` flag, a fast navigation leaks a subscription
that nothing will ever clean up.

### React StrictMode double-invokes effects

In development, `StrictMode` mounts, unmounts and remounts every component to surface
missing cleanup. Our effect subscribes, and the cleanup unsubscribes, so this is
harmless — and it is deliberately not worked around, because that double-invoke is
what proves the cleanup works. If you see duplicate subscriptions in development,
that is StrictMode telling you the cleanup is wrong.

### Stale closures

A handler captures the variables in scope when it was created. If `boardId` changes
and the subscription is not re-created, the handler keeps comparing against the old
one. Ours lists `[boardId, refresh]` as dependencies, so it re-subscribes when either
changes. `refresh` is wrapped in `useCallback` with an empty dependency list, so it is
stable and does not cause churn.

### Auto-cancellation

The SDK cancels an in-flight request when an identical one starts. That is a good
default for typeahead and a bad one here: a quick double-click on "Mark done" turns
into a confusing `autocancelled` error. It is off:

```ts
// frontend/src/lib/pocketbase.ts
pb.autoCancellation(false);
```

### Proxies that buffer

This one bites in production, not locally. A reverse proxy that buffers responses will
hold your SSE messages in a buffer waiting for the response to finish — which it never
does. The stream appears connected and silent. And a proxy with a short read timeout
will cut the connection every 60 seconds.

Both are configured in `deploy/k8s/ingress.yaml`:

```yaml
nginx.ingress.kubernetes.io/proxy-buffering: "off"
nginx.ingress.kubernetes.io/proxy-read-timeout: "3600"
nginx.ingress.kubernetes.io/proxy-send-timeout: "3600"
```

If realtime works locally but not behind your ingress, this is the first thing to
check.

### Sign-out

The realtime client carries the auth state it had when it subscribed. `pb.authStore`
changes trigger the SDK to re-authenticate the connection. Our `AuthProvider`
listens for auth changes and re-renders, which unmounts the board and its
subscription on sign-out.

## Debugging

**Is the connection open?** Browser dev tools, Network tab, filter on `realtime`. You
should see one `GET /api/realtime` that stays pending. Its "EventStream" or
"Response" tab shows messages as they arrive.

**Is the subscription registered?** In the same tab, look for a `POST /api/realtime`
returning 204. Its request body lists the topics. No POST means `subscribe` failed —
check the console.

**Are events being sent but ignored?** Log everything before the board filter:

```ts
pb.collection('occurrences').subscribe('*', (event) => {
  console.log('[realtime]', event.action, event.record?.id, 'board=', event.record?.board);
  // ...
});
```

If events arrive with a different `board` than the one you are viewing, the stream is
healthy and the filter is doing its job.

**Are events not arriving at all?** Reproduce with the curl walkthrough
[above](#seeing-it-for-yourself). If curl sees them and the browser does not, the
problem is client-side. If curl does not see them either, check that the token you
subscribed with belongs to a member of the relevant team — permission failures are
silent by design, since telling a client "you were denied an event" would itself leak
that the event happened.

**Server-side logging.** Run the backend with `--dev` for verbose logs, including
`Realtime connection established` with the client id.

## Extending it

### Live card edits

Currently, another person editing a card's instructions is not pushed. To add it,
subscribe to `cards` alongside `occurrences`:

```ts
useEffect(() => {
  if (!boardId) return;

  let disposed = false;
  const disposers: (() => void)[] = [];

  const add = (collection: string, matches: (record: any) => boolean) => {
    pb.collection(collection)
      .subscribe('*', (event) => {
        if (matches(event.record)) refresh();
      })
      .then((dispose) => {
        if (disposed) { void dispose(); return; }
        disposers.push(dispose);
      })
      .catch(() => {});
  };

  add('occurrences', (r) => r?.board === boardId);
  add('cards', (r) => r?.board === boardId);

  return () => {
    disposed = true;
    disposers.forEach((d) => void d());
  };
}, [boardId, refresh]);
```

`cards` has a `TeamMember` list rule, so the same scoping applies.

### Reducing traffic with a server-side filter

Move the board check to the server, as shown [above](#why-we-filter-by-board-on-the-client).
Worth doing if a team's boards get busy.

### Debouncing bursts

If several cards are completed in quick succession, each event triggers a refetch. A
small debounce collapses a burst into one request:

```ts
const debouncedRefresh = useMemo(() => {
  let timer: number | undefined;
  return () => {
    window.clearTimeout(timer);
    timer = window.setTimeout(refresh, 150);
  };
}, [refresh]);
```

Not currently needed, and worth adding only when you can measure the problem.

### Presence, typing indicators, and similar

Out of scope for this application, and they need a different shape: SSE only goes one
way, so anything where clients broadcast to each other wants WebSockets or a
server-side relay.

## Summary

| | |
| --- | --- |
| Transport | Server-sent events, one long-lived `GET /api/realtime` |
| Handshake | Stream yields a `clientId`; client POSTs its subscription list |
| We subscribe to | `occurrences`, collection-wide |
| Scoping | PocketBase applies the collection's ListRule per subscriber, so you get your teams only |
| On an event | Filter by `board`, then refetch the whole board |
| Why refetch | Missed events self-heal, and the server owns how a board is assembled |
| Not covered | Period rollovers, which involve no write and so produce no event; a timer handles those |
| Where | `frontend/src/boards/useBoardState.ts` |
