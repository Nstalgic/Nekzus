package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	bcryptCost        = 12
	minPasswordLength = 8
	minUsernameLength = 1
	maxUsernameLength = 255
)

// User represents a user account in the system
type User struct {
	ID           int        `json:"id"`
	Username     string     `json:"username"`
	PasswordHash string     `json:"-"` // Never serialize to JSON
	CreatedAt    time.Time  `json:"createdAt"`
	LastLogin    *time.Time `json:"lastLogin,omitempty"`
	IsActive     bool       `json:"isActive"`
}

// InitializeUserSchema creates the users table if it doesn't exist
func (s *Store) InitializeUserSchema() error {
	query := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		last_login DATETIME,
		is_active BOOLEAN DEFAULT 1
	)`

	_, err := s.db.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to create users table: %w", err)
	}

	// Create index on username for faster lookups
	indexQuery := `CREATE INDEX IF NOT EXISTS idx_users_username ON users(username)`
	_, err = s.db.Exec(indexQuery)
	if err != nil {
		return fmt.Errorf("failed to create username index: %w", err)
	}

	// Create index on is_active for filtering
	activeIndexQuery := `CREATE INDEX IF NOT EXISTS idx_users_active ON users(is_active)`
	_, err = s.db.Exec(activeIndexQuery)
	if err != nil {
		return fmt.Errorf("failed to create active index: %w", err)
	}

	return nil
}

// CreateUser creates a new user with a hashed password
func (s *Store) CreateUser(username, password string) error {
	// Validate inputs
	if err := validateUsername(username); err != nil {
		return fmt.Errorf("username validation failed: %w", err)
	}
	if err := validatePassword(password); err != nil {
		return fmt.Errorf("password validation failed: %w", err)
	}

	// Hash password
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Insert user
	query := `
		INSERT INTO users (username, password_hash)
		VALUES (?, ?)
	`

	_, err = s.db.Exec(query, username, string(hash))
	if err != nil {
		// Check for unique constraint violation
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return fmt.Errorf("user %s already exists", username)
		}
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
}

// GetUserByUsername retrieves a user by username
func (s *Store) GetUserByUsername(username string) (*User, error) {
	query := `
		SELECT id, username, password_hash, created_at, last_login, is_active
		FROM users
		WHERE username = ?
	`

	var user User
	var lastLogin sql.NullTime

	err := s.db.QueryRow(query, username).Scan(
		&user.ID,
		&user.Username,
		&user.PasswordHash,
		&user.CreatedAt,
		&lastLogin,
		&user.IsActive,
	)

	if err == sql.ErrNoRows {
		return nil, nil // User not found
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	if lastLogin.Valid {
		user.LastLogin = &lastLogin.Time
	}

	return &user, nil
}

// ValidateCredentials checks if the username and password are valid
func (s *Store) ValidateCredentials(username, password string) (*User, error) {
	// Get user
	user, err := s.GetUserByUsername(username)
	if err != nil {
		return nil, err
	}

	// User not found
	if user == nil {
		// Use constant-time comparison to prevent timing attacks
		// Hash a dummy password to maintain consistent timing
		bcrypt.CompareHashAndPassword([]byte("$2a$12$dummy"), []byte(password))
		return nil, errors.New("invalid username or password")
	}

	// Check if user is active
	if !user.IsActive {
		return nil, errors.New("account is disabled")
	}

	// Check password
	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		return nil, errors.New("invalid username or password")
	}

	return user, nil
}

// UpdateLastLogin updates the last_login timestamp for a user
func (s *Store) UpdateLastLogin(userID int) error {
	query := `
		UPDATE users
		SET last_login = CURRENT_TIMESTAMP
		WHERE id = ?
	`

	result, err := s.db.Exec(query, userID)
	if err != nil {
		return fmt.Errorf("failed to update last login: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("user with id %d not found", userID)
	}

	return nil
}

// ListUsers retrieves all users
func (s *Store) ListUsers() ([]User, error) {
	query := `
		SELECT id, username, password_hash, created_at, last_login, is_active
		FROM users
		ORDER BY created_at DESC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var user User
		var lastLogin sql.NullTime

		err := rows.Scan(
			&user.ID,
			&user.Username,
			&user.PasswordHash,
			&user.CreatedAt,
			&lastLogin,
			&user.IsActive,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user: %w", err)
		}

		if lastLogin.Valid {
			user.LastLogin = &lastLogin.Time
		}

		users = append(users, user)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating users: %w", err)
	}

	if users == nil {
		users = []User{} // Return empty slice, not nil
	}

	return users, nil
}

// DeleteUser removes a user from the database
func (s *Store) DeleteUser(userID int) error {
	query := `DELETE FROM users WHERE id = ?`

	result, err := s.db.Exec(query, userID)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("user with id %d not found", userID)
	}

	return nil
}

// validateUsername checks if username is valid
func validateUsername(username string) error {
	if len(username) < minUsernameLength {
		return errors.New("username is required")
	}
	if len(username) > maxUsernameLength {
		return fmt.Errorf("username must be at most %d characters", maxUsernameLength)
	}
	// Allow alphanumeric, underscore, hyphen, and dot
	for _, r := range username {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.') {
			return errors.New("username can only contain letters, numbers, underscore, hyphen, and dot")
		}
	}
	return nil
}

// validatePassword checks if password meets minimum requirements
func validatePassword(password string) error {
	if len(password) == 0 {
		return errors.New("password is required")
	}
	if len(password) < minPasswordLength {
		return fmt.Errorf("password must be at least %d characters", minPasswordLength)
	}
	return nil
}
