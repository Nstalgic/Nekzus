package storage

import (
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// setupTestDB creates a temporary database for testing
func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()

	// Create temp file
	tempFile, err := os.CreateTemp("", "users_test_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tempFile.Close()

	// Clean up after test
	t.Cleanup(func() {
		os.Remove(tempFile.Name())
	})

	// Open database
	db, err := sql.Open("sqlite3", tempFile.Name())
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	// Enable WAL mode
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		t.Fatalf("Failed to enable WAL: %v", err)
	}

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		t.Fatalf("Failed to enable foreign keys: %v", err)
	}

	return db
}

func TestInitializeUserSchema(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	store := &Store{db: db}

	// Test schema initialization
	err := store.InitializeUserSchema()
	if err != nil {
		t.Fatalf("InitializeUserSchema failed: %v", err)
	}

	// Verify table exists by querying it
	var tableName string
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='users'").Scan(&tableName)
	if err != nil {
		t.Fatalf("Users table not created: %v", err)
	}
	if tableName != "users" {
		t.Errorf("Expected table name 'users', got '%s'", tableName)
	}

	// Verify schema by trying to insert a user
	_, err = db.Exec("INSERT INTO users (username, password_hash) VALUES (?, ?)", "test", "hash123")
	if err != nil {
		t.Errorf("Failed to insert into users table: %v", err)
	}
}

func TestCreateUser(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	store := &Store{db: db}
	if err := store.InitializeUserSchema(); err != nil {
		t.Fatalf("Schema initialization failed: %v", err)
	}

	tests := []struct {
		name        string
		username    string
		password    string
		wantErr     bool
		errContains string
	}{
		{
			name:     "valid user",
			username: "admin",
			password: "SecureP@ssw0rd!",
			wantErr:  false,
		},
		{
			name:     "valid user with long password",
			username: "user2",
			password: "ThisIsAVeryLongPasswordWith123NumbersAndSpecialChars!@#$%",
			wantErr:  false,
		},
		{
			name:        "empty username",
			username:    "",
			password:    "password123",
			wantErr:     true,
			errContains: "username is required",
		},
		{
			name:        "empty password",
			username:    "user3",
			password:    "",
			wantErr:     true,
			errContains: "password is required",
		},
		{
			name:        "duplicate username",
			username:    "admin",
			password:    "DifferentPassword123!",
			wantErr:     true,
			errContains: "already exists",
		},
		{
			name:        "short password",
			username:    "user4",
			password:    "short",
			wantErr:     true,
			errContains: "password must be at least",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.CreateUser(tt.username, tt.password)

			if tt.wantErr {
				if err == nil {
					t.Errorf("CreateUser() expected error containing '%s', got nil", tt.errContains)
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("CreateUser() error = %v, want error containing %v", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("CreateUser() unexpected error: %v", err)
				}

				// Verify user was created
				user, err := store.GetUserByUsername(tt.username)
				if err != nil {
					t.Errorf("GetUserByUsername() error: %v", err)
				}
				if user == nil {
					t.Error("User not found after creation")
				} else {
					if user.Username != tt.username {
						t.Errorf("Username mismatch: got %v, want %v", user.Username, tt.username)
					}
					if user.PasswordHash == "" {
						t.Error("Password hash is empty")
					}
					if user.PasswordHash == tt.password {
						t.Error("Password stored in plaintext - should be hashed!")
					}
					if !user.IsActive {
						t.Error("New user should be active by default")
					}
					if user.CreatedAt.IsZero() {
						t.Error("CreatedAt timestamp not set")
					}
				}
			}
		})
	}
}

func TestGetUserByUsername(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	store := &Store{db: db}
	if err := store.InitializeUserSchema(); err != nil {
		t.Fatalf("Schema initialization failed: %v", err)
	}

	// Create a test user
	testUsername := "testuser"
	testPassword := "TestPassword123!"
	if err := store.CreateUser(testUsername, testPassword); err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	tests := []struct {
		name     string
		username string
		wantUser bool
		wantErr  bool
	}{
		{
			name:     "existing user",
			username: testUsername,
			wantUser: true,
			wantErr:  false,
		},
		{
			name:     "non-existent user",
			username: "doesnotexist",
			wantUser: false,
			wantErr:  false,
		},
		{
			name:     "empty username",
			username: "",
			wantUser: false,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := store.GetUserByUsername(tt.username)

			if tt.wantErr && err == nil {
				t.Error("Expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if tt.wantUser && user == nil {
				t.Error("Expected user, got nil")
			}
			if !tt.wantUser && user != nil {
				t.Errorf("Expected nil user, got %+v", user)
			}

			if user != nil {
				if user.Username != tt.username {
					t.Errorf("Username mismatch: got %v, want %v", user.Username, tt.username)
				}
				// Password hash should never be returned in JSON
				if user.PasswordHash == "" {
					t.Error("Password hash should be populated internally")
				}
			}
		})
	}
}

func TestValidateCredentials(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	store := &Store{db: db}
	if err := store.InitializeUserSchema(); err != nil {
		t.Fatalf("Schema initialization failed: %v", err)
	}

	// Create test users
	validUsername := "validuser"
	validPassword := "ValidPassword123!"
	if err := store.CreateUser(validUsername, validPassword); err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	// Create inactive user
	inactiveUsername := "inactiveuser"
	inactivePassword := "InactivePassword123!"
	if err := store.CreateUser(inactiveUsername, inactivePassword); err != nil {
		t.Fatalf("Failed to create inactive user: %v", err)
	}
	// Deactivate user directly via SQL
	if _, err := db.Exec("UPDATE users SET is_active = 0 WHERE username = ?", inactiveUsername); err != nil {
		t.Fatalf("Failed to deactivate user: %v", err)
	}

	tests := []struct {
		name        string
		username    string
		password    string
		wantUser    bool
		wantErr     bool
		errContains string
	}{
		{
			name:     "valid credentials",
			username: validUsername,
			password: validPassword,
			wantUser: true,
			wantErr:  false,
		},
		{
			name:        "wrong password",
			username:    validUsername,
			password:    "WrongPassword123!",
			wantUser:    false,
			wantErr:     true,
			errContains: "invalid username or password",
		},
		{
			name:        "non-existent user",
			username:    "nonexistent",
			password:    "SomePassword123!",
			wantUser:    false,
			wantErr:     true,
			errContains: "invalid username or password",
		},
		{
			name:        "inactive user",
			username:    inactiveUsername,
			password:    inactivePassword,
			wantUser:    false,
			wantErr:     true,
			errContains: "account is disabled",
		},
		{
			name:        "empty password",
			username:    validUsername,
			password:    "",
			wantUser:    false,
			wantErr:     true,
			errContains: "invalid username or password",
		},
		{
			name:        "empty username",
			username:    "",
			password:    validPassword,
			wantUser:    false,
			wantErr:     true,
			errContains: "invalid username or password",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := store.ValidateCredentials(tt.username, tt.password)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error containing '%s', got nil", tt.errContains)
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("Error = %v, want error containing %v", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}

			if tt.wantUser && user == nil {
				t.Error("Expected user, got nil")
			}
			if !tt.wantUser && user != nil {
				t.Errorf("Expected nil user, got %+v", user)
			}

			if user != nil && user.Username != tt.username {
				t.Errorf("Username mismatch: got %v, want %v", user.Username, tt.username)
			}
		})
	}
}

func TestUpdateLastLogin(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	store := &Store{db: db}
	if err := store.InitializeUserSchema(); err != nil {
		t.Fatalf("Schema initialization failed: %v", err)
	}

	// Create test user
	username := "testuser"
	password := "TestPassword123!"
	if err := store.CreateUser(username, password); err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	// Get user to get ID
	user, err := store.GetUserByUsername(username)
	if err != nil || user == nil {
		t.Fatalf("Failed to get user: %v", err)
	}

	// Initially last_login should be nil
	if user.LastLogin != nil {
		t.Error("LastLogin should be nil for new user")
	}

	// Update last login
	err = store.UpdateLastLogin(user.ID)
	if err != nil {
		t.Errorf("UpdateLastLogin failed: %v", err)
	}

	// Verify last_login was updated
	user, err = store.GetUserByUsername(username)
	if err != nil {
		t.Fatalf("Failed to get user after update: %v", err)
	}

	if user.LastLogin == nil {
		t.Error("LastLogin should be set after update")
	} else {
		// Check that timestamp is recent (within last 5 seconds)
		if time.Since(*user.LastLogin) > 5*time.Second {
			t.Errorf("LastLogin timestamp is not recent: %v", user.LastLogin)
		}
	}

	// Test updating non-existent user
	err = store.UpdateLastLogin(99999)
	if err == nil {
		t.Error("Expected error when updating non-existent user")
	}
}

func TestListUsers(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	store := &Store{db: db}
	if err := store.InitializeUserSchema(); err != nil {
		t.Fatalf("Schema initialization failed: %v", err)
	}

	// Initially should be empty
	users, err := store.ListUsers()
	if err != nil {
		t.Fatalf("ListUsers failed: %v", err)
	}
	if len(users) != 0 {
		t.Errorf("Expected 0 users, got %d", len(users))
	}

	// Create test users
	testUsers := []struct {
		username string
		password string
	}{
		{"admin", "AdminPass123!"},
		{"user1", "User1Pass123!"},
		{"user2", "User2Pass123!"},
	}

	for _, tu := range testUsers {
		if err := store.CreateUser(tu.username, tu.password); err != nil {
			t.Fatalf("Failed to create user %s: %v", tu.username, err)
		}
	}

	// List users
	users, err = store.ListUsers()
	if err != nil {
		t.Fatalf("ListUsers failed: %v", err)
	}

	if len(users) != len(testUsers) {
		t.Errorf("Expected %d users, got %d", len(testUsers), len(users))
	}

	// Verify usernames
	usernames := make(map[string]bool)
	for _, u := range users {
		usernames[u.Username] = true
		// Verify password hash is not empty
		if u.PasswordHash == "" {
			t.Errorf("User %s has empty password hash", u.Username)
		}
		// Verify all users are active
		if !u.IsActive {
			t.Errorf("User %s should be active", u.Username)
		}
	}

	for _, tu := range testUsers {
		if !usernames[tu.username] {
			t.Errorf("User %s not found in list", tu.username)
		}
	}
}

func TestDeleteUser(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	store := &Store{db: db}
	if err := store.InitializeUserSchema(); err != nil {
		t.Fatalf("Schema initialization failed: %v", err)
	}

	// Create test user
	username := "deleteme"
	password := "DeletePass123!"
	if err := store.CreateUser(username, password); err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	// Get user to get ID
	user, err := store.GetUserByUsername(username)
	if err != nil || user == nil {
		t.Fatalf("Failed to get user: %v", err)
	}
	userID := user.ID

	// Delete user
	err = store.DeleteUser(userID)
	if err != nil {
		t.Errorf("DeleteUser failed: %v", err)
	}

	// Verify user is deleted
	user, err = store.GetUserByUsername(username)
	if err != nil {
		t.Errorf("Error checking deleted user: %v", err)
	}
	if user != nil {
		t.Error("User should be deleted")
	}

	// Test deleting non-existent user
	err = store.DeleteUser(99999)
	if err == nil {
		t.Error("Expected error when deleting non-existent user")
	}
}

func TestUserPasswordHashing(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	store := &Store{db: db}
	if err := store.InitializeUserSchema(); err != nil {
		t.Fatalf("Schema initialization failed: %v", err)
	}

	username := "hashtest"
	password := "TestPassword123!"

	// Create user
	if err := store.CreateUser(username, password); err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Get user and check password hash
	user, err := store.GetUserByUsername(username)
	if err != nil || user == nil {
		t.Fatalf("Failed to get user: %v", err)
	}

	// Password hash should not equal plaintext password
	if user.PasswordHash == password {
		t.Error("Password is stored in plaintext - should be hashed!")
	}

	// Password hash should start with bcrypt prefix
	if !contains(user.PasswordHash, "$2a$") && !contains(user.PasswordHash, "$2b$") && !contains(user.PasswordHash, "$2y$") {
		t.Errorf("Password hash doesn't appear to be bcrypt format: %s", user.PasswordHash)
	}

	// Creating another user with same password should produce different hash
	username2 := "hashtest2"
	if err := store.CreateUser(username2, password); err != nil {
		t.Fatalf("Failed to create second user: %v", err)
	}

	user2, err := store.GetUserByUsername(username2)
	if err != nil || user2 == nil {
		t.Fatalf("Failed to get second user: %v", err)
	}

	if user.PasswordHash == user2.PasswordHash {
		t.Error("Same password should produce different hashes (salt should be random)")
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && indexOf(s, substr) >= 0))
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
