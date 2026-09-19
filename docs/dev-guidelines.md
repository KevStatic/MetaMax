# MetaMax — Development Guidelines

These guidelines exist to keep the codebase clean, fast to navigate, and easy for a 3-person team to work on in parallel without stepping on each other.

---

## 1. Module Boundaries

Each internal package owns its domain entirely. No package reaches into another's internals.

**Correct:**
```go
// api/handlers.go calls the container manager through its public interface
info, err := srv.mgr.Create(ctx, opts)
```

**Wrong:**
```go
// api/handlers.go directly calls Docker SDK — wrong layer
resp, err := dockerClient.ContainerCreate(ctx, ...)
```

**Rule:** If you need something from another module, add a method to that module's public interface. Never import a module's dependency directly from another module.

---

## 2. Directory Conventions

```
backend/internal/{module}/
    {module}.go     — main struct + constructor (NewXxx)
    {noun}.go       — domain type operations (e.g. luks.go, workspace.go)
    {noun}_test.go  — tests alongside the implementation
```

Each module exports exactly one primary struct (e.g. `Manager`, `Session`, `Scanner`, `Store`). Constructors are always named `New{Struct}`.

Integrations live outside `backend/` since they are consumed by the backend but may also be used independently:

```
integrations/
    0g/client.go
    axl/client.go
    keeperhub/client.go
```

Each integration exposes an interface, not just a concrete struct. This enables mocking in tests.

---

## 3. Error Handling

- Wrap errors with context: `fmt.Errorf("create container: %w", err)`
- Never swallow errors silently: `_ = someCall()` only for intentional cleanup (e.g. `defer cleanup()`)
- Fatal only in `main.go` during startup. All other code returns errors.
- User-facing errors (API responses) must not expose internal stack traces

```go
// correct — internal error wrapped with context
if err := mgr.Create(ctx, opts); err != nil {
    return fmt.Errorf("agent create_container: %w", err)
}

// wrong — raw error exposed to HTTP client
http.Error(w, err.Error(), 500)

// correct — generic message to client, detailed log server-side
log.Printf("create container: %v", err)
http.Error(w, "container creation failed", http.StatusInternalServerError)
```

---

## 4. Logging Standards

Use `log.Printf` for all server-side logging. No third-party logging library — keep it simple.

**Format:** `[module] action: detail`

```go
log.Printf("[agent] session %s: calling analyze_repo", s.ID)
log.Printf("[container] created %s (image=%s, ram=%dMB)", id[:12], opts.Image, opts.RAMMb)
log.Printf("[chain] attestation submitted: tx=%s", result.TxHash)
log.Printf("[luks] formatted and mounted at %s (mapper=%s)", mountPath, mapper)
```

Never log secrets, private keys, vault keys, or user data.

---

## 5. Configuration

All configuration comes from environment variables loaded in `config/config.go`. No hardcoded values anywhere else in the codebase.

```go
// correct
cfg.ProviderRegistryAddress

// wrong
"0xabcd1234..."   // hardcoded address in handler
```

Provide a `.env.example` for each service. Document every variable.

---

## 6. Keep It Minimal

This is a hackathon project. Features not in the checklist do not get built.

**Do not:**
- Add abstraction layers that are only used once
- Build generics or framework-level patterns for 2–3 use cases
- Add middleware, interceptors, or plugins "for future use"
- Implement features not on the checklist during the hackathon

**Do:**
- Write the simplest code that makes the checklist item work
- Add a `// TODO(post-hackathon):` comment for things you consciously skip
- Prefer explicit code over clever code

---

## 7. Concurrency

The backend runs one goroutine per agent session. Sessions do not share state.

```go
// Each session has its own channel — no shared mutex needed for events
s.events <- Event{Type: "action", Action: &action}
```

Only `container.Manager` uses a mutex (for its internal registries). If you add shared mutable state, document the locking strategy explicitly.

---

## 8. Integration Code

Integration clients (`integrations/0g`, `integrations/axl`, `integrations/keeperhub`) each follow the same pattern:

1. Define an interface at the top of `client.go`
2. Implement the interface with a concrete struct
3. Provide a `New(cfg Config) (InterfaceType, error)` constructor
4. Provide a `Noop()` implementation that silently does nothing — used when the integration is disabled

```go
type Client interface {
    Put(ctx context.Context, key string, value []byte) error
    Get(ctx context.Context, key string) ([]byte, error)
}

type noopClient struct{}
func (n *noopClient) Put(_ context.Context, _ string, _ []byte) error { return nil }
func (n *noopClient) Get(_ context.Context, _ string) ([]byte, error) { return nil, nil }

func Noop() Client { return &noopClient{} }
```

This means the backend can run without any integrations configured — they are opt-in at runtime via environment variables.

---

## 9. Frontend Conventions

- One route = one directory under `app/`
- API calls go through `lib/api.ts` only — no raw `fetch()` in components
- WebSocket connections managed in a custom hook, not inline in components
- Wallet state managed in `AuthContext` — no other component stores wallet address
- No inline styles — use Tailwind classes

---

## 10. Contract Conventions

- One contract per file
- NatSpec comments on all public functions
- Events for every state change
- Custom errors (not `require(false, "string")`)
- Constants for all magic numbers
- Test with Hardhat before deploying

---

## 11. Git Workflow

Three people, one branch per feature:

```
main                   ← stable, demo-ready
feature/agent-loop     ← one person's work
feature/luks-setup
feature/0g-integration
```

Commit messages: `module: short description`

```
agent: add confirm gate for deployment plan
container: implement LUKS teardown
chain: handle empty provider list gracefully
```

Merge to `main` only when the feature item is checked off in `docs/checklist.md`.

---

## 12. What Not to Build

The following are explicitly out of scope for the hackathon:

- Production-grade RBAC / permissions system
- Multi-tenancy isolation beyond team ID scoping
- Automated contract upgrades
- Rate limiting beyond x402 payment gating
- Frontend mobile responsiveness
- Observability / metrics stack (Prometheus, Grafana)
- CI/CD pipeline

These can be added after the hackathon. Don't let them block the demo.
