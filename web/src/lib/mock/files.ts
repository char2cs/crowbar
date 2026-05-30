// web/src/lib/mock/files.ts
import type { FileEntry } from '@/features/file-system/types/app'

export type GitStatus = 'modified' | 'added' | 'deleted' | 'renamed' | 'untracked'

/** Public shape used by tests and consumers. */
export type FileNode = FileEntry

function repoIdFromPath(rootPath: string): string {
  return rootPath.split('/').filter(Boolean).pop() ?? 'default'
}

const TREES: Record<string, FileEntry[]> = {
  crowbar: [
    { name: 'api', path: 'api', isDir: true, children: [
      { name: 'cmd', path: 'api/cmd', isDir: true, children: [
        { name: 'crowbar', path: 'api/cmd/crowbar', isDir: true, children: [
          { name: 'main.go', path: 'api/cmd/crowbar/main.go', isDir: false },
        ]},
      ]},
      { name: 'internal', path: 'api/internal', isDir: true, children: [
        { name: 'api', path: 'api/internal/api', isDir: true, children: [
          { name: 'v0', path: 'api/internal/api/v0', isDir: true, children: [
            { name: 'router.go', path: 'api/internal/api/v0/router.go', isDir: false, gitStatus: 'modified' },
            { name: 'workspaces_handler.go', path: 'api/internal/api/v0/workspaces_handler.go', isDir: false },
            { name: 'git_handler.go', path: 'api/internal/api/v0/git_handler.go', isDir: false, gitStatus: 'added' },
            { name: 'ws_git_handler.go', path: 'api/internal/api/v0/ws_git_handler.go', isDir: false, gitStatus: 'added' },
          ]},
          { name: 'middleware', path: 'api/internal/api/middleware', isDir: true, children: [
            { name: 'chaos.go', path: 'api/internal/api/middleware/chaos.go', isDir: false, gitStatus: 'added' },
          ]},
        ]},
        { name: 'fixtures', path: 'api/internal/fixtures', isDir: true, children: [
          { name: 'types.go', path: 'api/internal/fixtures/types.go', isDir: false },
          { name: 'store.go', path: 'api/internal/fixtures/store.go', isDir: false },
          { name: 'loader.go', path: 'api/internal/fixtures/loader.go', isDir: false },
          { name: 'workspaces.json', path: 'api/internal/fixtures/workspaces.json', isDir: false },
          { name: 'git-log.json', path: 'api/internal/fixtures/git-log.json', isDir: false, gitStatus: 'added' },
        ]},
      ]},
      { name: 'go.mod', path: 'api/go.mod', isDir: false },
    ]},
    { name: 'web', path: 'web', isDir: true, children: [
      { name: 'src', path: 'web/src', isDir: true, children: [
        { name: 'features', path: 'web/src/features', isDir: true, children: [
          { name: 'git', path: 'web/src/features/git', isDir: true, children: [
            { name: 'components', path: 'web/src/features/git/components', isDir: true, children: [
              { name: 'git-view.tsx', path: 'web/src/features/git/components/git-view.tsx', isDir: false, gitStatus: 'modified' },
            ]},
            { name: 'api', path: 'web/src/features/git/api', isDir: true, children: [
              { name: 'git-commits-api.ts', path: 'web/src/features/git/api/git-commits-api.ts', isDir: false, gitStatus: 'modified' },
              { name: 'git-status-api.ts', path: 'web/src/features/git/api/git-status-api.ts', isDir: false, gitStatus: 'modified' },
              { name: 'git-branches-api.ts', path: 'web/src/features/git/api/git-branches-api.ts', isDir: false, gitStatus: 'modified' },
            ]},
          ]},
          { name: 'settings', path: 'web/src/features/settings', isDir: true, children: [
            { name: 'components', path: 'web/src/features/settings/components', isDir: true, children: [
              { name: 'tabs', path: 'web/src/features/settings/components/tabs', isDir: true, children: [
                { name: 'developer-settings.tsx', path: 'web/src/features/settings/components/tabs/developer-settings.tsx', isDir: false, gitStatus: 'added' },
              ]},
            ]},
          ]},
        ]},
        { name: 'mocks', path: 'web/src/mocks', isDir: true, children: [
          { name: 'handlers', path: 'web/src/mocks/handlers', isDir: true, children: [
            { name: 'git.ts', path: 'web/src/mocks/handlers/git.ts', isDir: false, gitStatus: 'modified' },
            { name: 'workspaces.ts', path: 'web/src/mocks/handlers/workspaces.ts', isDir: false, gitStatus: 'modified' },
          ]},
          { name: 'browser.ts', path: 'web/src/mocks/browser.ts', isDir: false },
        ]},
      ]},
      { name: 'package.json', path: 'web/package.json', isDir: false },
      { name: 'vite.config.ts', path: 'web/vite.config.ts', isDir: false },
    ]},
    { name: 'README.md', path: 'README.md', isDir: false },
    { name: '.github', path: '.github', isDir: true, children: [
      { name: 'workflows', path: '.github/workflows', isDir: true, children: [
        { name: 'ci.yml', path: '.github/workflows/ci.yml', isDir: false },
      ]},
    ]},
  ],

  'quiver-core': [
    { name: 'src', path: 'src', isDir: true, children: [
      { name: 'auth', path: 'src/auth', isDir: true, children: [
        { name: 'oauth2.ts', path: 'src/auth/oauth2.ts', isDir: false, gitStatus: 'added' },
        { name: 'jwt.ts', path: 'src/auth/jwt.ts', isDir: false, gitStatus: 'modified' },
        { name: 'middleware.ts', path: 'src/auth/middleware.ts', isDir: false },
        { name: 'types.ts', path: 'src/auth/types.ts', isDir: false },
      ]},
      { name: 'api', path: 'src/api', isDir: true, children: [
        { name: 'routes.ts', path: 'src/api/routes.ts', isDir: false, gitStatus: 'modified' },
        { name: 'handlers', path: 'src/api/handlers', isDir: true, children: [
          { name: 'users.ts', path: 'src/api/handlers/users.ts', isDir: false },
          { name: 'projects.ts', path: 'src/api/handlers/projects.ts', isDir: false },
          { name: 'billing.ts', path: 'src/api/handlers/billing.ts', isDir: false },
        ]},
      ]},
      { name: 'db', path: 'src/db', isDir: true, children: [
        { name: 'schema.ts', path: 'src/db/schema.ts', isDir: false },
        { name: 'migrations', path: 'src/db/migrations', isDir: true, children: [
          { name: '001_initial.sql', path: 'src/db/migrations/001_initial.sql', isDir: false },
          { name: '002_add_oauth.sql', path: 'src/db/migrations/002_add_oauth.sql', isDir: false, gitStatus: 'added' },
        ]},
      ]},
      { name: 'lib', path: 'src/lib', isDir: true, children: [
        { name: 'redis.ts', path: 'src/lib/redis.ts', isDir: false, gitStatus: 'added' },
        { name: 'email.ts', path: 'src/lib/email.ts', isDir: false },
        { name: 'logger.ts', path: 'src/lib/logger.ts', isDir: false },
      ]},
      { name: 'index.ts', path: 'src/index.ts', isDir: false },
    ]},
    { name: 'tests', path: 'tests', isDir: true, children: [
      { name: 'auth.test.ts', path: 'tests/auth.test.ts', isDir: false, gitStatus: 'modified' },
      { name: 'api.test.ts', path: 'tests/api.test.ts', isDir: false },
    ]},
    { name: 'package.json', path: 'package.json', isDir: false },
    { name: 'tsconfig.json', path: 'tsconfig.json', isDir: false },
    { name: 'docker-compose.yml', path: 'docker-compose.yml', isDir: false },
    { name: '.env.example', path: '.env.example', isDir: false, gitStatus: 'untracked' },
  ],

  'quiver-desktop': [
    { name: 'src-tauri', path: 'src-tauri', isDir: true, children: [
      { name: 'src', path: 'src-tauri/src', isDir: true, children: [
        { name: 'main.rs', path: 'src-tauri/src/main.rs', isDir: false },
        { name: 'shell', path: 'src-tauri/src/shell', isDir: true, children: [
          { name: 'mod.rs', path: 'src-tauri/src/shell/mod.rs', isDir: false, gitStatus: 'added' },
          { name: 'pty.rs', path: 'src-tauri/src/shell/pty.rs', isDir: false, gitStatus: 'added' },
        ]},
        { name: 'fs', path: 'src-tauri/src/fs', isDir: true, children: [
          { name: 'mod.rs', path: 'src-tauri/src/fs/mod.rs', isDir: false },
          { name: 'watcher.rs', path: 'src-tauri/src/fs/watcher.rs', isDir: false, gitStatus: 'modified' },
        ]},
      ]},
      { name: 'Cargo.toml', path: 'src-tauri/Cargo.toml', isDir: false, gitStatus: 'modified' },
      { name: 'tauri.conf.json', path: 'src-tauri/tauri.conf.json', isDir: false },
    ]},
    { name: 'src', path: 'src', isDir: true, children: [
      { name: 'app', path: 'src/app', isDir: true, children: [
        { name: 'App.tsx', path: 'src/app/App.tsx', isDir: false },
        { name: 'shell', path: 'src/app/shell', isDir: true, children: [
          { name: 'ShellWindow.tsx', path: 'src/app/shell/ShellWindow.tsx', isDir: false, gitStatus: 'added' },
          { name: 'TerminalPane.tsx', path: 'src/app/shell/TerminalPane.tsx', isDir: false, gitStatus: 'added' },
        ]},
      ]},
      { name: 'bridge', path: 'src/bridge', isDir: true, children: [
        { name: 'shell.ts', path: 'src/bridge/shell.ts', isDir: false, gitStatus: 'added' },
        { name: 'fs.ts', path: 'src/bridge/fs.ts', isDir: false },
      ]},
    ]},
    { name: 'package.json', path: 'package.json', isDir: false },
    { name: 'vite.config.ts', path: 'vite.config.ts', isDir: false },
  ],

  'quiver-cloud': [
    { name: 'terraform', path: 'terraform', isDir: true, children: [
      { name: 'modules', path: 'terraform/modules', isDir: true, children: [
        { name: 'vpc', path: 'terraform/modules/vpc', isDir: true, children: [
          { name: 'main.tf', path: 'terraform/modules/vpc/main.tf', isDir: false },
          { name: 'variables.tf', path: 'terraform/modules/vpc/variables.tf', isDir: false },
        ]},
        { name: 'eks', path: 'terraform/modules/eks', isDir: true, children: [
          { name: 'main.tf', path: 'terraform/modules/eks/main.tf', isDir: false, gitStatus: 'modified' },
          { name: 'node_groups.tf', path: 'terraform/modules/eks/node_groups.tf', isDir: false, gitStatus: 'added' },
        ]},
      ]},
      { name: 'environments', path: 'terraform/environments', isDir: true, children: [
        { name: 'prod', path: 'terraform/environments/prod', isDir: true, children: [
          { name: 'main.tf', path: 'terraform/environments/prod/main.tf', isDir: false },
        ]},
        { name: 'staging', path: 'terraform/environments/staging', isDir: true, children: [
          { name: 'main.tf', path: 'terraform/environments/staging/main.tf', isDir: false, gitStatus: 'modified' },
        ]},
      ]},
    ]},
    { name: 'k8s', path: 'k8s', isDir: true, children: [
      { name: 'apps', path: 'k8s/apps', isDir: true, children: [
        { name: 'api.yaml', path: 'k8s/apps/api.yaml', isDir: false },
        { name: 'worker.yaml', path: 'k8s/apps/worker.yaml', isDir: false },
        { name: 'multi-tenant.yaml', path: 'k8s/apps/multi-tenant.yaml', isDir: false, gitStatus: 'added' },
      ]},
      { name: 'base', path: 'k8s/base', isDir: true, children: [
        { name: 'namespace.yaml', path: 'k8s/base/namespace.yaml', isDir: false },
        { name: 'rbac.yaml', path: 'k8s/base/rbac.yaml', isDir: false, gitStatus: 'modified' },
      ]},
    ]},
    { name: 'scripts', path: 'scripts', isDir: true, children: [
      { name: 'deploy.sh', path: 'scripts/deploy.sh', isDir: false },
      { name: 'rollback.sh', path: 'scripts/rollback.sh', isDir: false },
    ]},
    { name: '.github', path: '.github', isDir: true, children: [
      { name: 'workflows', path: '.github/workflows', isDir: true, children: [
        { name: 'deploy.yml', path: '.github/workflows/deploy.yml', isDir: false, gitStatus: 'modified' },
      ]},
    ]},
  ],
}

export function getMockFileTree(rootPath: string): FileEntry[] {
  const repoId = repoIdFromPath(rootPath)
  return TREES[repoId] ?? TREES['crowbar']
}

/** Realistic mock content for each file in the mock tree.
 *  Go files use real tab characters for indentation — intentional! */
const MOCK_CONTENT: Record<string, string> = {
  'cmd/server/main.go': `package main

import (
\t"context"
\t"fmt"
\t"log/slog"
\t"net/http"
\t"os"
\t"os/signal"
\t"syscall"
\t"time"

\t"github.com/rabbyte/crowbar/internal/auth"
\t"github.com/rabbyte/crowbar/internal/config"
\t"github.com/rabbyte/crowbar/internal/db"
\t"github.com/rabbyte/crowbar/internal/payment"
)

func main() {
\tcfg, err := config.Load()
\tif err != nil {
\t\tslog.Error("failed to load config", "error", err)
\t\tos.Exit(1)
\t}

\tpool, err := db.Connect(cfg.DatabaseURL)
\tif err != nil {
\t\tslog.Error("failed to connect to database", "error", err)
\t\tos.Exit(1)
\t}
\tdefer pool.Close()

\tauthSvc := auth.NewService(pool, cfg.JWTSecret)
\tpaymentSvc := payment.NewService(cfg.StripeSecretKey)

\tmux := http.NewServeMux()
\tmux.HandleFunc("POST /auth/login", authSvc.HandleLogin)
\tmux.HandleFunc("POST /auth/refresh", authSvc.HandleRefresh)
\tmux.HandleFunc("POST /webhooks/stripe", paymentSvc.HandleWebhook)

\t// Protected routes
\tprotected := auth.Middleware(authSvc)(mux)
\tmux.HandleFunc("GET /api/me", auth.RequireAuth(authSvc, handleMe))
\tmux.HandleFunc("POST /api/payments", auth.RequireAuth(authSvc, paymentSvc.HandleCreateIntent))

\tserver := &http.Server{
\t\tAddr:         fmt.Sprintf(":%d", cfg.Port),
\t\tHandler:      protected,
\t\tReadTimeout:  15 * time.Second,
\t\tWriteTimeout: 15 * time.Second,
\t\tIdleTimeout:  60 * time.Second,
\t}

\tgo func() {
\t\tslog.Info("server starting", "addr", server.Addr)
\t\tif err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
\t\t\tslog.Error("server error", "error", err)
\t\t\tos.Exit(1)
\t\t}
\t}()

\tquit := make(chan os.Signal, 1)
\tsignal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
\t<-quit

\tctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
\tdefer cancel()
\tif err := server.Shutdown(ctx); err != nil {
\t\tslog.Error("shutdown error", "error", err)
\t}
\tslog.Info("server stopped")
}

func handleMe(w http.ResponseWriter, r *http.Request) {
\tclaims := auth.ClaimsFromContext(r.Context())
\tw.Header().Set("Content-Type", "application/json")
\tfmt.Fprintf(w, \`{"user_id":%q,"email":%q,"role":%q}\`, claims.UserID, claims.Email, claims.Role)
}`,

  'internal/auth/service.go': `package auth

import (
\t"context"
\t"errors"
\t"time"

\t"github.com/golang-jwt/jwt/v5"
\t"github.com/jackc/pgx/v5/pgxpool"
\t"golang.org/x/crypto/bcrypt"
)

type Service struct {
\tdb        *pgxpool.Pool
\tjwtSecret []byte
}

type Claims struct {
\tjwt.RegisteredClaims
\tUserID string \`json:"user_id"\`
\tEmail  string \`json:"email"\`
\tRole   string \`json:"role"\`
}

type contextKey string

const claimsKey contextKey = "claims"

func NewService(db *pgxpool.Pool, secret string) *Service {
\treturn &Service{db: db, jwtSecret: []byte(secret)}
}

func (s *Service) Authenticate(ctx context.Context, email, password string) (string, error) {
\tvar id, hash, role string
\terr := s.db.QueryRow(ctx,
\t\t"SELECT id, password_hash, role FROM users WHERE email = $1",
\t\temail,
\t).Scan(&id, &hash, &role)
\tif err != nil {
\t\treturn "", errors.New("invalid credentials")
\t}

\tif err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
\t\treturn "", errors.New("invalid credentials")
\t}

\treturn s.signToken(id, email, role)
}

func (s *Service) signToken(userID, email, role string) (string, error) {
\tclaims := Claims{
\t\tRegisteredClaims: jwt.RegisteredClaims{
\t\t\tExpiresAt: jwt.NewNumericDate(time.Now().Add(7 * 24 * time.Hour)),
\t\t\tIssuedAt:  jwt.NewNumericDate(time.Now()),
\t\t},
\t\tUserID: userID,
\t\tEmail:  email,
\t\tRole:   role,
\t}
\treturn jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.jwtSecret)
}

func (s *Service) Verify(tokenStr string) (*Claims, error) {
\ttoken, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
\t\tif _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
\t\t\treturn nil, errors.New("unexpected signing method")
\t\t}
\t\treturn s.jwtSecret, nil
\t})
\tif err != nil || !token.Valid {
\t\treturn nil, errors.New("invalid token")
\t}
\treturn token.Claims.(*Claims), nil
}

func ClaimsFromContext(ctx context.Context) *Claims {
\tc, _ := ctx.Value(claimsKey).(*Claims)
\treturn c
}`,

  'internal/auth/middleware.go': `package auth

import (
\t"context"
\t"net/http"
\t"strings"
)

// Middleware returns an http.Handler that validates JWT tokens.
func Middleware(svc *Service) func(http.Handler) http.Handler {
\treturn func(next http.Handler) http.Handler {
\t\treturn http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
\t\t\ttoken := bearerToken(r)
\t\t\tif token == "" {
\t\t\t\tnext.ServeHTTP(w, r)
\t\t\t\treturn
\t\t\t}
\t\t\tclaims, err := svc.Verify(token)
\t\t\tif err != nil {
\t\t\t\tnext.ServeHTTP(w, r)
\t\t\t\treturn
\t\t\t}
\t\t\tctx := context.WithValue(r.Context(), claimsKey, claims)
\t\t\tnext.ServeHTTP(w, r.WithContext(ctx))
\t\t})
\t}
}

// RequireAuth wraps a handler and returns 401 if no valid claims are present.
func RequireAuth(svc *Service, h http.HandlerFunc) http.HandlerFunc {
\treturn func(w http.ResponseWriter, r *http.Request) {
\t\tif ClaimsFromContext(r.Context()) == nil {
\t\t\thttp.Error(w, \`{"error":"unauthorized"}\`, http.StatusUnauthorized)
\t\t\treturn
\t\t}
\t\th(w, r)
\t}
}

func bearerToken(r *http.Request) string {
\th := r.Header.Get("Authorization")
\tif !strings.HasPrefix(h, "Bearer ") {
\t\treturn ""
\t}
\treturn strings.TrimPrefix(h, "Bearer ")
}

func (s *Service) HandleLogin(w http.ResponseWriter, r *http.Request) {
\tvar body struct {
\t\tEmail    string \`json:"email"\`
\t\tPassword string \`json:"password"\`
\t}
\tif err := decodeJSON(r, &body); err != nil {
\t\thttp.Error(w, \`{"error":"bad request"}\`, http.StatusBadRequest)
\t\treturn
\t}
\ttoken, err := s.Authenticate(r.Context(), body.Email, body.Password)
\tif err != nil {
\t\thttp.Error(w, \`{"error":"invalid credentials"}\`, http.StatusUnauthorized)
\t\treturn
\t}
\twriteJSON(w, http.StatusOK, map[string]string{"token": token})
}

func (s *Service) HandleRefresh(w http.ResponseWriter, r *http.Request) {
\tclaims := ClaimsFromContext(r.Context())
\tif claims == nil {
\t\thttp.Error(w, \`{"error":"unauthorized"}\`, http.StatusUnauthorized)
\t\treturn
\t}
\ttoken, err := s.signToken(claims.UserID, claims.Email, claims.Role)
\tif err != nil {
\t\thttp.Error(w, \`{"error":"internal error"}\`, http.StatusInternalServerError)
\t\treturn
\t}
\twriteJSON(w, http.StatusOK, map[string]string{"token": token})
}`,

  'internal/payment/service.go': `package payment

import (
\t"errors"
\t"net/http"

\t"github.com/stripe/stripe-go/v76"
\t"github.com/stripe/stripe-go/v76/paymentintent"
)

type Service struct {
\tstripeKey string
}

func NewService(stripeKey string) *Service {
\tstripe.Key = stripeKey
\treturn &Service{stripeKey: stripeKey}
}

type CreateIntentRequest struct {
\tAmount   int64  \`json:"amount"\`
\tCurrency string \`json:"currency"\`
\tOrderID  string \`json:"order_id"\`
}

func (s *Service) HandleCreateIntent(w http.ResponseWriter, r *http.Request) {
\tvar req CreateIntentRequest
\tif err := decodeJSON(r, &req); err != nil {
\t\thttp.Error(w, \`{"error":"bad request"}\`, http.StatusBadRequest)
\t\treturn
\t}
\tif req.Amount <= 0 {
\t\thttp.Error(w, \`{"error":"amount must be positive"}\`, http.StatusBadRequest)
\t\treturn
\t}

\tparams := &stripe.PaymentIntentParams{
\t\tAmount:   stripe.Int64(req.Amount),
\t\tCurrency: stripe.String(req.Currency),
\t\tMetadata: map[string]string{
\t\t\t"order_id": req.OrderID,
\t\t},
\t\tAutomaticPaymentMethods: &stripe.PaymentIntentAutomaticPaymentMethodsParams{
\t\t\tEnabled: stripe.Bool(true),
\t\t},
\t}

\tintent, err := paymentintent.New(params)
\tif err != nil {
\t\tvar stripeErr *stripe.Error
\t\tif errors.As(err, &stripeErr) {
\t\t\thttp.Error(w, \`{"error":"\`+stripeErr.Msg+\`"}\`, http.StatusBadGateway)
\t\t\treturn
\t\t}
\t\thttp.Error(w, \`{"error":"internal error"}\`, http.StatusInternalServerError)
\t\treturn
\t}

\twriteJSON(w, http.StatusCreated, map[string]string{
\t\t"client_secret": intent.ClientSecret,
\t\t"intent_id":     intent.ID,
\t})
}`,

  'internal/payment/webhook.go': `package payment

import (
\t"encoding/json"
\t"io"
\t"log/slog"
\t"net/http"

\t"github.com/stripe/stripe-go/v76"
\t"github.com/stripe/stripe-go/v76/webhook"
)

const maxBodyBytes = int64(65536)

func (s *Service) HandleWebhook(w http.ResponseWriter, r *http.Request) {
\tr.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

\tpayload, err := io.ReadAll(r.Body)
\tif err != nil {
\t\thttp.Error(w, "payload too large", http.StatusRequestEntityTooLarge)
\t\treturn
\t}

\tsig := r.Header.Get("Stripe-Signature")
\tevent, err := webhook.ConstructEvent(payload, sig, stripe.Key)
\tif err != nil {
\t\tslog.Warn("webhook signature verification failed", "error", err)
\t\thttp.Error(w, "invalid signature", http.StatusBadRequest)
\t\treturn
\t}

\tswitch event.Type {
\tcase stripe.EventTypePaymentIntentSucceeded:
\t\tvar intent stripe.PaymentIntent
\t\tif err := json.Unmarshal(event.Data.Raw, &intent); err != nil {
\t\t\tslog.Error("unmarshal payment intent", "error", err)
\t\t\tbreak
\t\t}
\t\tslog.Info("payment succeeded", "intent_id", intent.ID, "amount", intent.Amount)

\tcase stripe.EventTypePaymentIntentPaymentFailed:
\t\tvar intent stripe.PaymentIntent
\t\tif err := json.Unmarshal(event.Data.Raw, &intent); err != nil {
\t\t\tslog.Error("unmarshal payment intent", "error", err)
\t\t\tbreak
\t\t}
\t\tslog.Warn("payment failed",
\t\t\t"intent_id", intent.ID,
\t\t\t"reason", intent.LastPaymentError.Msg,
\t\t)
\t}

\twriteJSON(w, http.StatusOK, map[string]bool{"received": true})
}`,

  'internal/db/db.go': `package db

import (
\t"context"
\t"fmt"
\t"time"

\t"github.com/jackc/pgx/v5/pgxpool"
)

func Connect(url string) (*pgxpool.Pool, error) {
\tcfg, err := pgxpool.ParseConfig(url)
\tif err != nil {
\t\treturn nil, fmt.Errorf("parse db url: %w", err)
\t}

\tcfg.MaxConns = 20
\tcfg.MinConns = 2
\tcfg.MaxConnLifetime = 1 * time.Hour
\tcfg.MaxConnIdleTime = 30 * time.Minute
\tcfg.HealthCheckPeriod = 1 * time.Minute

\tpool, err := pgxpool.NewWithConfig(context.Background(), cfg)
\tif err != nil {
\t\treturn nil, fmt.Errorf("create pool: %w", err)
\t}

\tif err := pool.Ping(context.Background()); err != nil {
\t\treturn nil, fmt.Errorf("ping db: %w", err)
\t}

\treturn pool, nil
}`,

  'internal/db/models.go': `package db

import "time"

type Role string

const (
\tRoleUser  Role = "user"
\tRoleAdmin Role = "admin"
)

type User struct {
\tID           string    \`db:"id"\`
\tEmail        string    \`db:"email"\`
\tPasswordHash string    \`db:"password_hash"\`
\tRole         Role      \`db:"role"\`
\tCreatedAt    time.Time \`db:"created_at"\`
\tUpdatedAt    time.Time \`db:"updated_at"\`
}

type Session struct {
\tID        string    \`db:"id"\`
\tUserID    string    \`db:"user_id"\`
\tToken     string    \`db:"token"\`
\tExpiresAt time.Time \`db:"expires_at"\`
\tCreatedAt time.Time \`db:"created_at"\`
}

type PaymentIntent struct {
\tID         string    \`db:"id"\`
\tUserID     string    \`db:"user_id"\`
\tStripeID   string    \`db:"stripe_id"\`
\tAmount     int64     \`db:"amount"\`
\tCurrency   string    \`db:"currency"\`
\tStatus     string    \`db:"status"\`
\tOrderID    string    \`db:"order_id"\`
\tCreatedAt  time.Time \`db:"created_at"\`
}`,

  'internal/config/config.go': `package config

import (
\t"fmt"
\t"os"
\t"strconv"
)

type Config struct {
\tDatabaseURL     string
\tJWTSecret       string
\tStripeSecretKey string
\tWebhookSecret   string
\tPort            int
\tAllowedOrigins  []string
}

func Load() (*Config, error) {
\tport, err := strconv.Atoi(getenv("PORT", "8080"))
\tif err != nil {
\t\treturn nil, fmt.Errorf("invalid PORT: %w", err)
\t}

\tcfg := &Config{
\t\tDatabaseURL:     require("DATABASE_URL"),
\t\tJWTSecret:       require("JWT_SECRET"),
\t\tStripeSecretKey: require("STRIPE_SECRET_KEY"),
\t\tWebhookSecret:   require("STRIPE_WEBHOOK_SECRET"),
\t\tPort:            port,
\t}
\tif cfg.DatabaseURL == "" || cfg.JWTSecret == "" {
\t\treturn nil, fmt.Errorf("missing required environment variables")
\t}
\treturn cfg, nil
}

func require(key string) string {
\treturn os.Getenv(key)
}

func getenv(key, fallback string) string {
\tif v := os.Getenv(key); v != "" {
\t\treturn v
\t}
\treturn fallback
}`,

  'go.mod': `module github.com/rabbyte/crowbar

go 1.22

require (
\tgithub.com/golang-jwt/jwt/v5 v5.2.1
\tgithub.com/jackc/pgx/v5 v5.5.4
\tgithub.com/stripe/stripe-go/v76 v76.18.0
\tgolang.org/x/crypto v0.21.0
)

require (
\tgithub.com/jackc/pgpassfile v1.0.0 // indirect
\tgithub.com/jackc/pgservicefile v0.0.0-20231201235250-de7065d787c7 // indirect
\tgithub.com/jackc/puddle/v2 v2.2.1 // indirect
\tgolang.org/x/sync v0.6.0 // indirect
\tgolang.org/x/text v0.14.0 // indirect
)`,

  'go.sum': `github.com/golang-jwt/jwt/v5 v5.2.1 h1:OuVbFODueb089Lh128TAcimifWaLhJwVflnrgM17wHk=
github.com/golang-jwt/jwt/v5 v5.2.1/go.mod h1:pqrtFR0X4osieyHYxtmOUWsAWrfe1Q5UVIyoH402zdk=
github.com/jackc/pgpassfile v1.0.0 h1:/6Hmqy13Ss2zCq62VdNG8yXl6vi81tthe7ium9kK/N4=
github.com/jackc/pgpassfile v1.0.0/go.mod h1:CEx0iS5ambNFdcRtxPj5JhEz+xB6uRky5eyVu/W2HEg=
github.com/jackc/pgservicefile v0.0.0-20231201235250-de7065d787c7 h1:bbPeKD0xmW/Y25WS6uQEDPhHpDbG6Nxl9oBzRwYcHK0=
github.com/jackc/pgx/v5 v5.5.4 h1:Bh0MslJOHmmDFUxsHoHKjbHoXBNam1AJEV6HHKNTq0o=
github.com/jackc/pgx/v5 v5.5.4/go.mod h1:ez9gd+YPOCUTcjvUPQFfGBMfZEJeqEQmCkRSFVHUnR4=
github.com/stripe/stripe-go/v76 v76.18.0 h1:kJIWIQ4GGOL8eFaZK7TEr0lC6qBL7m5lMGWBzFGjYms=
github.com/stripe/stripe-go/v76 v76.18.0/go.mod h1:gVHlAj2dLUxwQAmrjBa4nqBWMRq5R5qFLYn1LFR4K7M=
golang.org/x/crypto v0.21.0 h1:X31++rzVUdKhX5sWmSOFZxx8UW/ldWx8S1QFfgRRd0E=
golang.org/x/crypto v0.21.0/go.mod h1:0BP7YvVV9gBbVKyeTG0Gyn+gZm94bibOW5BjDEYAOMs=`,

  'Makefile': `.PHONY: build run test lint migrate

build:
\tgo build -o bin/server ./cmd/server

run:
\tgo run ./cmd/server

test:
\tgo test ./... -v -race -count=1

lint:
\tgolangci-lint run ./...

migrate:
\tgoose -dir migrations postgres "$(DATABASE_URL)" up

migrate-down:
\tgoose -dir migrations postgres "$(DATABASE_URL)" down

tidy:
\tgo mod tidy

generate:
\tgo generate ./...`,

  'README.md': `# Crowbar API

Go backend service for the Crowbar agentic IDE platform.

## Requirements

- Go 1.22+
- PostgreSQL 15+
- Stripe account

## Setup

\`\`\`bash
cp .env.example .env
# Edit .env with your credentials

make migrate
make run
\`\`\`

## API

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | /auth/login | — | Exchange credentials for JWT |
| POST | /auth/refresh | Bearer | Refresh JWT |
| GET | /api/me | Bearer | Current user info |
| POST | /api/payments | Bearer | Create payment intent |
| POST | /webhooks/stripe | — | Stripe webhook handler |

## Development

\`\`\`bash
make test    # run all tests with race detector
make lint    # run golangci-lint
make build   # compile to bin/server
\`\`\``,

  '.env.example': `DATABASE_URL=postgres://localhost:5432/crowbar_dev?sslmode=disable
JWT_SECRET=change-me-in-production
STRIPE_SECRET_KEY=sk_test_...
STRIPE_WEBHOOK_SECRET=whsec_...
PORT=8080`,
}

const DEFAULT_CONTENT = `// File content not available in mock mode`

export function getMockFileContent(path: string): string {
  // Strip leading repo prefix (e.g. '/repos/crowbar/')
  const normalizedPath = path.replace(/^\/repos\/[^/]+\//, '')
  return MOCK_CONTENT[normalizedPath] ?? MOCK_CONTENT[path] ?? DEFAULT_CONTENT
}
