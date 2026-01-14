package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

var (
	ErrUserNotFound       = errors.New("user not found")
	ErrUserAlreadyExists  = errors.New("user already exists")
	ErrInvalidCredentials = errors.New("invalid credentials")
)

// CreateUser creates a new user
func (r *Repository) CreateUser(ctx context.Context, user *User) error {
	rolesJSON, err := json.Marshal(user.Roles)
	if err != nil {
		return fmt.Errorf("marshal roles: %w", err)
	}

	err = r.pool.QueryRow(ctx, `
		INSERT INTO users (email, name, avatar_url, password_hash, default_team_id, roles, email_verified, is_admin)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at, updated_at
	`, user.Email, user.Name, user.AvatarURL, user.PasswordHash, user.DefaultTeamID,
		rolesJSON, user.EmailVerified, user.IsAdmin,
	).Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)

	if err != nil {
		if isDuplicateKeyError(err) {
			return ErrUserAlreadyExists
		}
		return fmt.Errorf("insert user: %w", err)
	}

	return nil
}

// GetUserByID retrieves a user by ID
func (r *Repository) GetUserByID(ctx context.Context, id string) (*User, error) {
	return r.scanUser(ctx, `
		SELECT id, email, name, avatar_url, password_hash, default_team_id, roles, 
		       email_verified, is_admin, created_at, updated_at
		FROM users
		WHERE id = $1
	`, id)
}

// GetUserByEmail retrieves a user by email
func (r *Repository) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	return r.scanUser(ctx, `
		SELECT id, email, name, avatar_url, password_hash, default_team_id, roles, 
		       email_verified, is_admin, created_at, updated_at
		FROM users
		WHERE email = $1
	`, email)
}

// UpdateUser updates a user
func (r *Repository) UpdateUser(ctx context.Context, user *User) error {
	rolesJSON, err := json.Marshal(user.Roles)
	if err != nil {
		return fmt.Errorf("marshal roles: %w", err)
	}

	result, err := r.pool.Exec(ctx, `
		UPDATE users
		SET name = $2, avatar_url = $3, default_team_id = $4, roles = $5, 
		    email_verified = $6, is_admin = $7
		WHERE id = $1
	`, user.ID, user.Name, user.AvatarURL, user.DefaultTeamID,
		rolesJSON, user.EmailVerified, user.IsAdmin)

	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	return nil
}

// UpdateUserPassword updates a user's password
func (r *Repository) UpdateUserPassword(ctx context.Context, userID, passwordHash string) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE users SET password_hash = $2 WHERE id = $1
	`, userID, passwordHash)

	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	return nil
}

// DeleteUser deletes a user
func (r *Repository) DeleteUser(ctx context.Context, id string) error {
	result, err := r.pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	return nil
}

// CountUsers returns the total number of users
func (r *Repository) CountUsers(ctx context.Context) (int64, error) {
	var count int64
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return count, nil
}

// scanUser scans a user from a query
func (r *Repository) scanUser(ctx context.Context, query string, args ...any) (*User, error) {
	var user User
	var rolesJSON []byte

	err := r.pool.QueryRow(ctx, query, args...).Scan(
		&user.ID, &user.Email, &user.Name, &user.AvatarURL,
		&user.PasswordHash, &user.DefaultTeamID, &rolesJSON,
		&user.EmailVerified, &user.IsAdmin, &user.CreatedAt, &user.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("query user: %w", err)
	}

	if len(rolesJSON) > 0 {
		if err := json.Unmarshal(rolesJSON, &user.Roles); err != nil {
			return nil, fmt.Errorf("unmarshal roles: %w", err)
		}
	}

	return &user, nil
}

// isDuplicateKeyError checks if the error is a duplicate key violation
func isDuplicateKeyError(err error) bool {
	return err != nil && (err.Error() == "ERROR: duplicate key value violates unique constraint" ||
		contains(err.Error(), "duplicate key") ||
		contains(err.Error(), "23505"))
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsImpl(s, substr))
}

func containsImpl(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
