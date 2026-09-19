package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// --- Domain types ---

type Team struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Wallet    string    `json:"wallet"`
	CreatedAt time.Time `json:"created_at"`
}

type Session struct {
	ID             string    `json:"id"`
	TeamID         string    `json:"team_id"`
	Prompt         string    `json:"prompt"`
	State          string    `json:"state"`
	MerkleRoot     string    `json:"merkle_root"`
	AttestationTx  string    `json:"attestation_tx"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// ActionLog persists the complete action log for a session as a JSONB array.
type ActionLog struct {
	ID        int64     `json:"id"`
	SessionID string    `json:"session_id"`
	TeamID    string    `json:"team_id"`
	Actions   []byte    `json:"actions"` // JSONB array of agent.Action
	CreatedAt time.Time `json:"created_at"`
}

// Attestation records the EAS attestation submitted at session end.
type Attestation struct {
	ID             int64     `json:"id"`
	SessionID      string    `json:"session_id"`
	TxHash         string    `json:"tx_hash"`
	AttestationUID string    `json:"attestation_uid"`
	MerkleRoot     string    `json:"merkle_root"`
	SchemaUID      string    `json:"schema_uid"`
	EASScanURL     string    `json:"eas_scan_url"`
	CreatedAt      time.Time `json:"created_at"`
}

// ProviderRecord records the provider selected for a session.
type ProviderRecord struct {
	Address       string    `json:"address"`
	Endpoint      string    `json:"endpoint"`
	PricePerHour  string    `json:"price_per_hour"` // wei as string
	JobsCompleted int64     `json:"jobs_completed"`
	SelectedAt    time.Time `json:"selected_at"`
	SessionID     string    `json:"session_id"`
}

// Container tracks a provisioned Docker container.
type Container struct {
	ID          string    `json:"id"`
	TeamID      string    `json:"team_id"`
	SessionID   string    `json:"session_id"`
	DockerID    string    `json:"docker_id"`
	Name        string    `json:"name"`
	Image       string    `json:"image"`
	Status      string    `json:"status"`
	PortsJSON   string    `json:"ports_json"` // JSON object: containerPort → hostPort
	StoragePath string    `json:"storage_path"`
	CreatedAt   time.Time `json:"created_at"`
}

// Secret stores an encrypted key-value secret for a team.
type Secret struct {
	ID        int64     `json:"id"`
	TeamID    string    `json:"team_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// Project represents a linked GitHub repository with its deployment config.
type Project struct {
	ID             string    `json:"id"`
	TeamID         string    `json:"team_id"`
	Name           string    `json:"name"`
	RepoURL        string    `json:"repo_url"`
	Branch         string    `json:"branch"`
	LastPrompt     string    `json:"last_prompt"`
	WebhookSecret  string    `json:"webhook_secret"`
	AutoDeploy     bool      `json:"auto_deploy"`
	LastDeployedAt *time.Time `json:"last_deployed_at,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// ProjectEnvVar is one encrypted environment variable scoped to a project.
type ProjectEnvVar struct {
	ID        int64     `json:"id"`
	ProjectID string    `json:"project_id"`
	TeamID    string    `json:"team_id"`
	Key       string    `json:"key"`
	// Value is not included in list responses — use GetProjectEnvVarValues.
	CreatedAt time.Time `json:"created_at"`
}

// Payment records an x402 micro-payment.
type Payment struct {
	ID         int64     `json:"id"`
	Wallet     string    `json:"wallet"`
	SessionID  string    `json:"session_id"`
	AmountUSDC string    `json:"amount_usdc"`
	TxHash     string    `json:"tx_hash"`
	Nonce      string    `json:"nonce"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

// --- Store ---

type Store struct {
	pool *pgxpool.Pool
}

// New creates a Store and verifies database connectivity.
func New(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("pgx pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("db ping: %w", err)
	}
	return &Store{pool: pool}, nil
}

// Migrate creates all tables idempotently.
func (s *Store) Migrate(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS teams (
			id         TEXT PRIMARY KEY,
			name       TEXT NOT NULL,
			wallet     TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE UNIQUE INDEX IF NOT EXISTS teams_wallet_idx ON teams(wallet) WHERE wallet != '';

		CREATE TABLE IF NOT EXISTS sessions (
			id              TEXT PRIMARY KEY,
			team_id         TEXT NOT NULL REFERENCES teams(id),
			prompt          TEXT NOT NULL,
			state           TEXT NOT NULL DEFAULT 'running',
			merkle_root     TEXT NOT NULL DEFAULT '',
			attestation_tx  TEXT NOT NULL DEFAULT '',
			created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS action_logs (
			id         BIGSERIAL PRIMARY KEY,
			session_id TEXT NOT NULL REFERENCES sessions(id),
			team_id    TEXT NOT NULL,
			actions    JSONB NOT NULL DEFAULT '[]',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS attestations (
			id               BIGSERIAL PRIMARY KEY,
			session_id       TEXT NOT NULL REFERENCES sessions(id),
			tx_hash          TEXT NOT NULL,
			attestation_uid  TEXT NOT NULL DEFAULT '',
			merkle_root      TEXT NOT NULL DEFAULT '',
			schema_uid       TEXT NOT NULL DEFAULT '',
			eas_scan_url     TEXT NOT NULL DEFAULT '',
			created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS provider_selections (
			id             BIGSERIAL PRIMARY KEY,
			session_id     TEXT NOT NULL REFERENCES sessions(id),
			address        TEXT NOT NULL,
			endpoint       TEXT NOT NULL,
			price_per_hour TEXT NOT NULL DEFAULT '0',
			jobs_completed BIGINT NOT NULL DEFAULT 0,
			selected_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS containers (
			id           TEXT PRIMARY KEY,
			team_id      TEXT NOT NULL REFERENCES teams(id),
			session_id   TEXT NOT NULL DEFAULT '',
			docker_id    TEXT NOT NULL DEFAULT '',
			name         TEXT NOT NULL,
			image        TEXT NOT NULL,
			status       TEXT NOT NULL DEFAULT 'running',
			ports_json   TEXT NOT NULL DEFAULT '{}',
			storage_path TEXT NOT NULL DEFAULT '',
			created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS payments (
			id           BIGSERIAL PRIMARY KEY,
			wallet       TEXT NOT NULL,
			session_id   TEXT NOT NULL DEFAULT '',
			amount_usdc  TEXT NOT NULL DEFAULT '0',
			tx_hash      TEXT NOT NULL DEFAULT '',
			nonce        TEXT NOT NULL DEFAULT '',
			status       TEXT NOT NULL DEFAULT 'confirmed',
			created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE UNIQUE INDEX IF NOT EXISTS payments_nonce_idx ON payments(nonce) WHERE nonce != '';

		CREATE TABLE IF NOT EXISTS secrets (
			id         BIGSERIAL PRIMARY KEY,
			team_id    TEXT NOT NULL REFERENCES teams(id),
			name       TEXT NOT NULL,
			value      TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE UNIQUE INDEX IF NOT EXISTS secrets_team_name_idx ON secrets(team_id, name);

		CREATE TABLE IF NOT EXISTS projects (
			id               TEXT PRIMARY KEY,
			team_id          TEXT NOT NULL REFERENCES teams(id),
			name             TEXT NOT NULL,
			repo_url         TEXT NOT NULL DEFAULT '',
			branch           TEXT NOT NULL DEFAULT 'main',
			last_prompt      TEXT NOT NULL DEFAULT '',
			webhook_secret   TEXT NOT NULL DEFAULT '',
			auto_deploy      BOOLEAN NOT NULL DEFAULT FALSE,
			last_deployed_at TIMESTAMPTZ,
			created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS projects_team_idx ON projects(team_id);

		CREATE TABLE IF NOT EXISTS project_env_vars (
			id         BIGSERIAL PRIMARY KEY,
			project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
			team_id    TEXT NOT NULL REFERENCES teams(id),
			key        TEXT NOT NULL,
			value      TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE(project_id, key)
		);
		CREATE INDEX IF NOT EXISTS project_env_vars_project_idx ON project_env_vars(project_id);
	`)
	return err
}

// --- Teams ---

func (s *Store) CreateTeam(ctx context.Context, t *Team) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO teams (id, name, wallet, created_at) VALUES ($1, $2, $3, $4)`,
		t.ID, t.Name, t.Wallet, t.CreatedAt)
	return err
}

func (s *Store) GetTeam(ctx context.Context, id string) (*Team, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, name, COALESCE(wallet,''), created_at FROM teams WHERE id = $1`, id)
	var t Team
	if err := row.Scan(&t.ID, &t.Name, &t.Wallet, &t.CreatedAt); err != nil {
		return nil, err
	}
	return &t, nil
}

// GetOrCreateTeamByWallet finds or creates the team for a given wallet address.
func (s *Store) GetOrCreateTeamByWallet(ctx context.Context, wallet string) (*Team, error) {
	w := strings.ToLower(wallet)
	row := s.pool.QueryRow(ctx,
		`SELECT id, name, COALESCE(wallet,''), created_at FROM teams WHERE wallet = $1`, w)
	var t Team
	err := row.Scan(&t.ID, &t.Name, &t.Wallet, &t.CreatedAt)
	if err == nil {
		return &t, nil
	}
	now := time.Now().UTC()
	suffix := w
	if len(w) > 10 {
		suffix = w[2:10]
	}
	t = Team{
		ID:        "team-" + suffix,
		Name:      "account-" + suffix,
		Wallet:    w,
		CreatedAt: now,
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO teams (id, name, wallet, created_at) VALUES ($1,$2,$3,$4)
		 ON CONFLICT (wallet) WHERE wallet != '' DO NOTHING`,
		t.ID, t.Name, t.Wallet, t.CreatedAt)
	if err != nil {
		return nil, err
	}
	row = s.pool.QueryRow(ctx,
		`SELECT id, name, COALESCE(wallet,''), created_at FROM teams WHERE wallet = $1`, w)
	if err2 := row.Scan(&t.ID, &t.Name, &t.Wallet, &t.CreatedAt); err2 != nil {
		return nil, err2
	}
	return &t, nil
}

// --- Sessions ---

func (s *Store) CreateSession(ctx context.Context, sess *Session) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO sessions (id, team_id, prompt, state, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		sess.ID, sess.TeamID, sess.Prompt, sess.State, sess.CreatedAt, sess.UpdatedAt)
	return err
}

func (s *Store) UpdateSessionState(ctx context.Context, id, state string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE sessions SET state=$1, updated_at=NOW() WHERE id=$2`, state, id)
	return err
}

func (s *Store) UpdateSessionMerkleRoot(ctx context.Context, id, merkleRoot, attestationTx string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE sessions SET merkle_root=$1, attestation_tx=$2, updated_at=NOW() WHERE id=$3`,
		merkleRoot, attestationTx, id)
	return err
}

func (s *Store) GetSession(ctx context.Context, id string) (*Session, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, team_id, prompt, state, merkle_root, attestation_tx, created_at, updated_at
		 FROM sessions WHERE id=$1`, id)
	var sess Session
	err := row.Scan(&sess.ID, &sess.TeamID, &sess.Prompt, &sess.State,
		&sess.MerkleRoot, &sess.AttestationTx, &sess.CreatedAt, &sess.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &sess, nil
}

func (s *Store) ListTeamSessions(ctx context.Context, teamID string) ([]Session, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, team_id, prompt, state, merkle_root, attestation_tx, created_at, updated_at
		 FROM sessions WHERE team_id=$1 ORDER BY created_at DESC`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		var sess Session
		if err := rows.Scan(&sess.ID, &sess.TeamID, &sess.Prompt, &sess.State,
			&sess.MerkleRoot, &sess.AttestationTx, &sess.CreatedAt, &sess.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	return out, nil
}

// --- Action Logs ---

func (s *Store) UpsertActionLog(ctx context.Context, sessionID, teamID string, actions any) error {
	data, err := json.Marshal(actions)
	if err != nil {
		return fmt.Errorf("marshal actions: %w", err)
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO action_logs (session_id, team_id, actions)
		VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING`, sessionID, teamID, data)
	if err != nil {
		// Try update if insert conflicted
		_, err = s.pool.Exec(ctx,
			`UPDATE action_logs SET actions=$1 WHERE session_id=$2`, data, sessionID)
	}
	return err
}

func (s *Store) GetActionLog(ctx context.Context, sessionID string) (*ActionLog, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, session_id, team_id, actions, created_at FROM action_logs WHERE session_id=$1`, sessionID)
	var al ActionLog
	if err := row.Scan(&al.ID, &al.SessionID, &al.TeamID, &al.Actions, &al.CreatedAt); err != nil {
		return nil, err
	}
	return &al, nil
}

// --- Attestations ---

func (s *Store) CreateAttestation(ctx context.Context, a *Attestation) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO attestations (session_id, tx_hash, attestation_uid, merkle_root, schema_uid, eas_scan_url)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		a.SessionID, a.TxHash, a.AttestationUID, a.MerkleRoot, a.SchemaUID, a.EASScanURL)
	return err
}

func (s *Store) GetAttestation(ctx context.Context, sessionID string) (*Attestation, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, session_id, tx_hash, attestation_uid, merkle_root, schema_uid, eas_scan_url, created_at
		 FROM attestations WHERE session_id=$1 ORDER BY id DESC LIMIT 1`, sessionID)
	var a Attestation
	if err := row.Scan(&a.ID, &a.SessionID, &a.TxHash, &a.AttestationUID,
		&a.MerkleRoot, &a.SchemaUID, &a.EASScanURL, &a.CreatedAt); err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Store) ListTeamAttestations(ctx context.Context, teamID string) ([]Attestation, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT a.id, a.session_id, a.tx_hash, a.attestation_uid, a.merkle_root, a.schema_uid, a.eas_scan_url, a.created_at
		FROM attestations a
		JOIN sessions s ON s.id = a.session_id
		WHERE s.team_id = $1
		ORDER BY a.created_at DESC`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Attestation
	for rows.Next() {
		var a Attestation
		if err := rows.Scan(&a.ID, &a.SessionID, &a.TxHash, &a.AttestationUID,
			&a.MerkleRoot, &a.SchemaUID, &a.EASScanURL, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

// --- Provider Selections ---

func (s *Store) RecordProviderSelection(ctx context.Context, p *ProviderRecord) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO provider_selections (session_id, address, endpoint, price_per_hour, jobs_completed, selected_at)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		p.SessionID, p.Address, p.Endpoint, p.PricePerHour, p.JobsCompleted, p.SelectedAt)
	return err
}

// --- Containers ---

func (s *Store) CreateContainer(ctx context.Context, c *Container) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO containers (id, team_id, session_id, docker_id, name, image, status, ports_json, storage_path, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		c.ID, c.TeamID, c.SessionID, c.DockerID, c.Name, c.Image, c.Status, c.PortsJSON, c.StoragePath, c.CreatedAt)
	return err
}

func (s *Store) UpdateContainerStatus(ctx context.Context, id, status string) error {
	_, err := s.pool.Exec(ctx, `UPDATE containers SET status=$1 WHERE id=$2`, status, id)
	return err
}

func (s *Store) ListTeamContainers(ctx context.Context, teamID string) ([]Container, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, team_id, session_id, docker_id, name, image, status, ports_json, storage_path, created_at
		 FROM containers WHERE team_id=$1 ORDER BY created_at DESC`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Container
	for rows.Next() {
		var c Container
		if err := rows.Scan(&c.ID, &c.TeamID, &c.SessionID, &c.DockerID,
			&c.Name, &c.Image, &c.Status, &c.PortsJSON, &c.StoragePath, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// --- Payments ---

func (s *Store) CreatePayment(ctx context.Context, p *Payment) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO payments (wallet, session_id, amount_usdc, tx_hash, nonce, status)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		p.Wallet, p.SessionID, p.AmountUSDC, p.TxHash, p.Nonce, p.Status)
	return err
}

// PaymentNonceExists returns true if the given nonce has already been recorded.
// Used to prevent replay attacks.
func (s *Store) PaymentNonceExists(ctx context.Context, nonce string) (bool, error) {
	var count int
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM payments WHERE nonce=$1`, nonce).Scan(&count)
	return count > 0, err
}

func (s *Store) ListPaymentsByWallet(ctx context.Context, wallet string) ([]Payment, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, wallet, session_id, amount_usdc, tx_hash, nonce, status, created_at
		 FROM payments WHERE wallet=$1 ORDER BY created_at DESC`, strings.ToLower(wallet))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Payment
	for rows.Next() {
		var p Payment
		if err := rows.Scan(&p.ID, &p.Wallet, &p.SessionID, &p.AmountUSDC,
			&p.TxHash, &p.Nonce, &p.Status, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// --- Secrets ---

func (s *Store) CreateSecret(ctx context.Context, teamID, name, value string) (*Secret, error) {
	var id int64
	var createdAt time.Time
	err := s.pool.QueryRow(ctx,
		`INSERT INTO secrets (team_id, name, value) VALUES ($1,$2,$3) RETURNING id, created_at`,
		teamID, name, value).Scan(&id, &createdAt)
	if err != nil {
		return nil, err
	}
	return &Secret{ID: id, TeamID: teamID, Name: name, CreatedAt: createdAt}, nil
}

func (s *Store) ListSecrets(ctx context.Context, teamID string) ([]Secret, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, team_id, name, created_at FROM secrets WHERE team_id=$1 ORDER BY created_at DESC`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Secret
	for rows.Next() {
		var sec Secret
		if err := rows.Scan(&sec.ID, &sec.TeamID, &sec.Name, &sec.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, sec)
	}
	return out, nil
}

func (s *Store) DeleteSecret(ctx context.Context, id int64, teamID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM secrets WHERE id=$1 AND team_id=$2`, id, teamID)
	return err
}

// GetSecretValue retrieves the encrypted value for a secret (used internally by container provisioning).
func (s *Store) GetSecretValue(ctx context.Context, id int64, teamID string) (string, error) {
	var value string
	err := s.pool.QueryRow(ctx,
		`SELECT value FROM secrets WHERE id=$1 AND team_id=$2`, id, teamID).Scan(&value)
	return value, err
}

// UpdateTeamName changes the display name for a team.
func (s *Store) UpdateTeamName(ctx context.Context, teamID, name string) error {
	_, err := s.pool.Exec(ctx, `UPDATE teams SET name=$1 WHERE id=$2`, name, teamID)
	return err
}

// --- Projects ---

func (s *Store) CreateProject(ctx context.Context, p *Project) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO projects (id, team_id, name, repo_url, branch, last_prompt, webhook_secret, auto_deploy, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		p.ID, p.TeamID, p.Name, p.RepoURL, p.Branch, p.LastPrompt, p.WebhookSecret, p.AutoDeploy, p.CreatedAt)
	return err
}

func (s *Store) ListProjects(ctx context.Context, teamID string) ([]Project, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, team_id, name, repo_url, branch, last_prompt, webhook_secret, auto_deploy, last_deployed_at, created_at
		 FROM projects WHERE team_id=$1 ORDER BY created_at DESC`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.TeamID, &p.Name, &p.RepoURL, &p.Branch,
			&p.LastPrompt, &p.WebhookSecret, &p.AutoDeploy, &p.LastDeployedAt, &p.CreatedAt); err != nil {
			return nil, err
		}
		p.WebhookSecret = "" // never return raw secret in list
		out = append(out, p)
	}
	return out, nil
}

func (s *Store) GetProject(ctx context.Context, id, teamID string) (*Project, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, team_id, name, repo_url, branch, last_prompt, webhook_secret, auto_deploy, last_deployed_at, created_at
		 FROM projects WHERE id=$1 AND team_id=$2`, id, teamID)
	var p Project
	if err := row.Scan(&p.ID, &p.TeamID, &p.Name, &p.RepoURL, &p.Branch,
		&p.LastPrompt, &p.WebhookSecret, &p.AutoDeploy, &p.LastDeployedAt, &p.CreatedAt); err != nil {
		return nil, err
	}
	return &p, nil
}

// GetProjectByWebhook looks up a project by ID without team scoping — used only for webhook verification.
func (s *Store) GetProjectByWebhook(ctx context.Context, id string) (*Project, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, team_id, name, repo_url, branch, last_prompt, webhook_secret, auto_deploy, last_deployed_at, created_at
		 FROM projects WHERE id=$1`, id)
	var p Project
	if err := row.Scan(&p.ID, &p.TeamID, &p.Name, &p.RepoURL, &p.Branch,
		&p.LastPrompt, &p.WebhookSecret, &p.AutoDeploy, &p.LastDeployedAt, &p.CreatedAt); err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) UpdateProject(ctx context.Context, id, teamID, name, repoURL, branch, lastPrompt string, autoDeploy bool) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE projects SET name=$1, repo_url=$2, branch=$3, last_prompt=$4, auto_deploy=$5
		 WHERE id=$6 AND team_id=$7`,
		name, repoURL, branch, lastPrompt, autoDeploy, id, teamID)
	return err
}

func (s *Store) TouchProjectDeployedAt(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE projects SET last_deployed_at=NOW() WHERE id=$1`, id)
	return err
}

func (s *Store) DeleteProject(ctx context.Context, id, teamID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM projects WHERE id=$1 AND team_id=$2`, id, teamID)
	return err
}

// --- Project Env Vars ---

// UpsertProjectEnvVar inserts or updates an env var for a project (encrypted value expected from caller).
func (s *Store) UpsertProjectEnvVar(ctx context.Context, projectID, teamID, key, encryptedValue string) (*ProjectEnvVar, error) {
	var id int64
	var createdAt time.Time
	err := s.pool.QueryRow(ctx,
		`INSERT INTO project_env_vars (project_id, team_id, key, value)
		 VALUES ($1,$2,$3,$4)
		 ON CONFLICT (project_id, key) DO UPDATE SET value=EXCLUDED.value
		 RETURNING id, created_at`,
		projectID, teamID, key, encryptedValue).Scan(&id, &createdAt)
	if err != nil {
		return nil, err
	}
	return &ProjectEnvVar{ID: id, ProjectID: projectID, TeamID: teamID, Key: key, CreatedAt: createdAt}, nil
}

// ListProjectEnvVars returns keys only (no values) for display in the UI.
func (s *Store) ListProjectEnvVars(ctx context.Context, projectID, teamID string) ([]ProjectEnvVar, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, project_id, team_id, key, created_at FROM project_env_vars
		 WHERE project_id=$1 AND team_id=$2 ORDER BY key ASC`, projectID, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ProjectEnvVar
	for rows.Next() {
		var v ProjectEnvVar
		if err := rows.Scan(&v.ID, &v.ProjectID, &v.TeamID, &v.Key, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// GetProjectEnvVarValues returns the encrypted values for all env vars of a project.
// The caller (handler) must decrypt each value using the vault master secret.
func (s *Store) GetProjectEnvVarValues(ctx context.Context, projectID, teamID string) (map[string]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT key, value FROM project_env_vars WHERE project_id=$1 AND team_id=$2`, projectID, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, nil
}

func (s *Store) DeleteProjectEnvVar(ctx context.Context, id int64, projectID, teamID string) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM project_env_vars WHERE id=$1 AND project_id=$2 AND team_id=$3`, id, projectID, teamID)
	return err
}

